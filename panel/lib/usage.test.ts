import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { findDeviceUsage, sumMonthUsage, sumServerUsage } from './usage'
import { USAGE_DAYS } from './usage-days'
import { fakeCollection, fakeCursor } from './fake-collection'
import { collection } from './mongodb'

vi.mock('./mongodb', () => ({ collection: vi.fn() }))

const dailyUsage = fakeCollection()

const NOW = new Date('2026-09-03T12:00:00Z')
const FIRST_DAY = '2026-08-05'
const TODAY = '2026-09-03'

function findReturning(docs: unknown[]) {
  const cursor = fakeCursor(docs)
  dailyUsage.find.mockReturnValue(cursor)
  return cursor
}

function aggregateReturning(docs: unknown[]) {
  dailyUsage.aggregate.mockReturnValue({ toArray: vi.fn(async () => docs) })
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.mocked(collection).mockResolvedValue(dailyUsage.asCollection())
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  dailyUsage.find.mockReset()
  dailyUsage.aggregate.mockReset()
})

describe('findDeviceUsage', () => {
  it('queries the device identity over the last USAGE_DAYS UTC days, oldest first', async () => {
    const cursor = findReturning([])

    await findDeviceUsage('device-1')

    expect(collection).toHaveBeenCalledWith('daily_usage')
    expect(dailyUsage.find).toHaveBeenCalledWith({
      identity: 'device-1',
      day: { $gte: FIRST_DAY },
    })
    expect(cursor.sort).toHaveBeenCalledWith({ day: 1 })
  })

  it('always returns USAGE_DAYS days, filling days without traffic with zeros', async () => {
    findReturning([
      { day: FIRST_DAY, rxBytes: 100, txBytes: 10 },
      { day: TODAY, rxBytes: 300 },
    ])

    const usage = await findDeviceUsage('device-1')

    expect(USAGE_DAYS).toBe(30)
    expect(usage?.days).toHaveLength(USAGE_DAYS)
    expect(usage?.days[0]).toEqual({ day: FIRST_DAY, rxBytes: 100, txBytes: 10 })
    expect(usage?.days[1]).toEqual({ day: '2026-08-06', rxBytes: 0, txBytes: 0 })
    expect(usage?.days[USAGE_DAYS - 1]).toEqual({ day: TODAY, rxBytes: 300, txBytes: 0 })
    expect(usage?.totalRx).toBe(400)
    expect(usage?.totalTx).toBe(10)
  })

  it('ignores rows outside the window', async () => {
    findReturning([{ day: '2026-08-04', rxBytes: 999, txBytes: 999 }])

    const usage = await findDeviceUsage('device-1')

    expect(usage?.totalRx).toBe(0)
    expect(usage?.totalTx).toBe(0)
  })

  it('returns null when the database fails', async () => {
    vi.mocked(collection).mockRejectedValueOnce(new Error('atlas down'))

    expect(await findDeviceUsage('device-1')).toBeNull()
    expect(console.error).toHaveBeenCalled()
  })
})

describe('sumServerUsage', () => {
  it('sums every identity per day over the window', async () => {
    aggregateReturning([
      { _id: '2026-09-01', rxBytes: 1000, txBytes: 200 },
      { _id: TODAY, rxBytes: 500, txBytes: 50 },
    ])

    const usage = await sumServerUsage()

    expect(dailyUsage.aggregate).toHaveBeenCalledWith([
      { $match: { day: { $gte: FIRST_DAY } } },
      { $group: { _id: '$day', rxBytes: { $sum: '$rxBytes' }, txBytes: { $sum: '$txBytes' } } },
      { $sort: { _id: 1 } },
    ])
    expect(usage?.days).toHaveLength(USAGE_DAYS)
    expect(usage?.days[USAGE_DAYS - 3]).toEqual({ day: '2026-09-01', rxBytes: 1000, txBytes: 200 })
    expect(usage?.days[USAGE_DAYS - 1]).toEqual({ day: TODAY, rxBytes: 500, txBytes: 50 })
    expect(usage?.totalRx).toBe(1500)
    expect(usage?.totalTx).toBe(250)
  })

  it('returns null when the database fails', async () => {
    dailyUsage.aggregate.mockImplementation(() => {
      throw new Error('atlas down')
    })

    expect(await sumServerUsage()).toBeNull()
  })
})

describe('sumMonthUsage', () => {
  it('sums every identity per day from the 1st of the current UTC month to today', async () => {
    aggregateReturning([
      { _id: '2026-09-01', rxBytes: 7000, txBytes: 700 },
      { _id: TODAY, rxBytes: 3000, txBytes: 300 },
    ])

    const month = await sumMonthUsage()

    expect(dailyUsage.aggregate).toHaveBeenCalledWith([
      { $match: { day: { $gte: '2026-09-01' } } },
      { $group: { _id: '$day', rxBytes: { $sum: '$rxBytes' }, txBytes: { $sum: '$txBytes' } } },
      { $sort: { _id: 1 } },
    ])
    expect(month?.days).toEqual([
      { day: '2026-09-01', rxBytes: 7000, txBytes: 700 },
      { day: '2026-09-02', rxBytes: 0, txBytes: 0 },
      { day: TODAY, rxBytes: 3000, txBytes: 300 },
    ])
    expect(month?.totalRx).toBe(10000)
    expect(month?.totalTx).toBe(1000)
  })

  it.each([
    ['2026-09-01T00:30:00Z', '2026-09-01', 1],
    ['2026-09-23T12:00:00Z', '2026-09-23', 23],
    ['2026-08-31T23:59:00Z', '2026-08-31', 31],
  ])('on %s spans the month up to %s', async (now, lastDay, length) => {
    vi.setSystemTime(new Date(now))
    aggregateReturning([])

    const month = await sumMonthUsage()

    expect(month?.days).toHaveLength(length)
    expect(month?.days[0].day).toBe(`${lastDay.slice(0, 7)}-01`)
    expect(month?.days.at(-1)?.day).toBe(lastDay)
  })

  it('returns null when the database fails', async () => {
    vi.mocked(collection).mockRejectedValueOnce(new Error('atlas down'))

    expect(await sumMonthUsage()).toBeNull()
  })
})
