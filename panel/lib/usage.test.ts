import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import usageFixture from '../../testdata/contracts/panel-usage.json'
import { readProvisiond } from './provisiond'
import { findDeviceUsage, findServerUsage, type UsageResponse } from './usage'
import { USAGE_DAYS } from './usage-days'

vi.mock('./provisiond', () => ({ readProvisiond: vi.fn() }))

const NOW = new Date('2026-09-03T12:00:00Z')
const FIRST_DAY = '2026-08-05'
const TODAY = '2026-09-03'

function answering(days: UsageResponse['days']) {
  vi.mocked(readProvisiond).mockResolvedValue({ days })
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.mocked(readProvisiond).mockReset()
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('findDeviceUsage', () => {
  it('asks for the device over the last USAGE_DAYS UTC days', async () => {
    answering([])

    await findDeviceUsage('dev 1')

    expect(readProvisiond).toHaveBeenCalledWith(`/admin/usage?from=${FIRST_DAY}&device=dev+1`)
  })

  it('always returns USAGE_DAYS days, filling days without traffic with zeros', async () => {
    answering([{ day: TODAY, rx_bytes: 300, tx_bytes: 30 }])

    const usage = await findDeviceUsage('dev1')

    expect(usage?.days).toHaveLength(USAGE_DAYS)
    expect(usage?.days[0]).toEqual({ day: FIRST_DAY, rxBytes: 0, txBytes: 0 })
    expect(usage?.days.at(-1)).toEqual({ day: TODAY, rxBytes: 300, txBytes: 30 })
    expect(usage?.totalRx).toBe(300)
    expect(usage?.totalTx).toBe(30)
  })

  it('reads the wire shape provisiond sends', async () => {
    vi.setSystemTime(new Date('2026-09-12T12:00:00Z'))
    vi.mocked(readProvisiond).mockResolvedValue(usageFixture)

    const usage = await findDeviceUsage('dev1')

    expect(usage?.days.slice(-2)).toEqual([
      { day: '2026-09-11', rxBytes: 1048576, txBytes: 524288 },
      { day: '2026-09-12', rxBytes: 2097152, txBytes: 0 },
    ])
  })

  it('returns null when provisiond fails', async () => {
    vi.mocked(readProvisiond).mockRejectedValue(new Error('down'))
    expect(await findDeviceUsage('dev1')).toBeNull()
  })
})

describe('findServerUsage', () => {
  it('reads the month and the history in one request from the earlier first day', async () => {
    answering([
      { day: '2026-08-31', rx_bytes: 5000, tx_bytes: 500 },
      { day: '2026-09-01', rx_bytes: 7000, tx_bytes: 700 },
      { day: TODAY, rx_bytes: 3000, tx_bytes: 300 },
    ])

    const { month, history } = await findServerUsage()

    expect(readProvisiond).toHaveBeenCalledTimes(1)
    expect(readProvisiond).toHaveBeenCalledWith(`/admin/usage?from=${FIRST_DAY}`)
    expect(month?.days).toEqual([
      { day: '2026-09-01', rxBytes: 7000, txBytes: 700 },
      { day: '2026-09-02', rxBytes: 0, txBytes: 0 },
      { day: TODAY, rxBytes: 3000, txBytes: 300 },
    ])
    expect(month?.totalRx).toBe(10000)
    expect(history?.days).toHaveLength(USAGE_DAYS)
    expect(history?.totalRx).toBe(15000)
  })

  it('starts at the 1st when the month is longer than the history', async () => {
    vi.setSystemTime(new Date('2026-08-31T23:59:00Z'))
    answering([])

    const { month, history } = await findServerUsage()

    expect(readProvisiond).toHaveBeenCalledWith('/admin/usage?from=2026-08-01')
    expect(month?.days).toHaveLength(31)
    expect(history?.days).toHaveLength(USAGE_DAYS)
  })

  it.each([
    ['2026-09-01T00:30:00Z', '2026-09-01', 1],
    ['2026-09-23T12:00:00Z', '2026-09-23', 23],
  ])('on %s spans the month up to %s', async (now, lastDay, length) => {
    vi.setSystemTime(new Date(now))
    answering([])

    const { month } = await findServerUsage()

    expect(month?.days).toHaveLength(length)
    expect(month?.days[0].day).toBe(`${lastDay.slice(0, 7)}-01`)
    expect(month?.days.at(-1)?.day).toBe(lastDay)
  })

  it('returns null for both when provisiond fails', async () => {
    vi.mocked(readProvisiond).mockRejectedValue(new Error('down'))
    expect(await findServerUsage()).toEqual({ month: null, history: null })
  })
})
