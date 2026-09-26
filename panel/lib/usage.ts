import { collection } from './mongodb'
import { COLLECTION, FIELD, fieldRef } from './mongo-names'
import { USAGE_DAYS } from './usage-days'

export interface DailyUsageDocument {
  /** `<identity>:<YYYY-MM-DD>`. */
  [FIELD.id]: string
  /** The device id; `peer:<key>` for the legacy global token. */
  [FIELD.identity]: string
  [FIELD.day]: string
  [FIELD.rxBytes]: number
  [FIELD.txBytes]: number
  [FIELD.onlineSeconds]: number
}

function dailyUsageCollection() {
  return collection<DailyUsageDocument>(COLLECTION.dailyUsage)
}

// Sums the per-device deltas the collector stores, not WireGuard counters, which reset on every
// lease; that is what lets a device's history span its leases.

interface DailyUsage {
  /** `YYYY-MM-DD`. */
  day: string
  rxBytes: number
  txBytes: number
}

export interface UsageSummary {
  /** Every day of the requested range, oldest first, with zeros for days without traffic. */
  days: DailyUsage[]
  totalRx: number
  totalTx: number
}

/** `null` when the database didn't answer, so the page says so instead of showing zeros. */
export type UsageFromDatabase = UsageSummary | null

function isoDay(date: Date): string {
  return date.toISOString().slice(0, 10)
}

/** From `first` to today, in UTC like the collector's days. */
function daysSince(first: Date): string[] {
  const today = isoDay(new Date())
  const days: string[] = []

  for (const d = new Date(first); isoDay(d) <= today; d.setUTCDate(d.getUTCDate() + 1)) {
    days.push(isoDay(d))
  }
  return days
}

function lastDays(): string[] {
  const first = new Date()
  first.setUTCDate(first.getUTCDate() - (USAGE_DAYS - 1))
  return daysSince(first)
}

function monthDays(): string[] {
  const first = new Date()
  first.setUTCDate(1)
  return daysSince(first)
}

function summarize(days: string[], rows: DailyUsage[]): UsageSummary {
  const byDay = new Map(rows.map((row) => [row.day, row]))
  const filled = days.map((day) => byDay.get(day) ?? { day, rxBytes: 0, txBytes: 0 })

  return {
    days: filled,
    totalRx: filled.reduce((a, d) => a + d.rxBytes, 0),
    totalTx: filled.reduce((a, d) => a + d.txBytes, 0),
  }
}

/** `identity` is the device id. */
export async function findDeviceUsage(identity: string): Promise<UsageFromDatabase> {
  try {
    const dailyUsage = await dailyUsageCollection()
    const days = lastDays()

    const docs = await dailyUsage
      .find({ [FIELD.identity]: identity, [FIELD.day]: { $gte: days[0] } })
      .sort({ [FIELD.day]: 1 })
      .toArray()

    return summarize(
      days,
      docs.map((d) => ({
        day: d[FIELD.day],
        rxBytes: d[FIELD.rxBytes] ?? 0,
        txBytes: d[FIELD.txBytes] ?? 0,
      })),
    )
  } catch (error) {
    console.error('failed to read device usage', error)
    return null
  }
}

export function sumServerUsage(): Promise<UsageFromDatabase> {
  return sumServerDays(lastDays())
}

/** From the 1st of the current UTC month to today. */
export function sumMonthUsage(): Promise<UsageFromDatabase> {
  return sumServerDays(monthDays())
}

async function sumServerDays(days: string[]): Promise<UsageFromDatabase> {
  try {
    const dailyUsage = await dailyUsageCollection()

    const docs = await dailyUsage
      .aggregate<{ _id: string; rxBytes: number; txBytes: number }>([
        { $match: { [FIELD.day]: { $gte: days[0] } } },
        {
          $group: {
            _id: fieldRef(FIELD.day),
            rxBytes: { $sum: fieldRef(FIELD.rxBytes) },
            txBytes: { $sum: fieldRef(FIELD.txBytes) },
          },
        },
        { $sort: { _id: 1 } },
      ])
      .toArray()

    return summarize(
      days,
      docs.map((d) => ({ day: d._id, rxBytes: d.rxBytes, txBytes: d.txBytes })),
    )
  } catch (error) {
    console.error('failed to read server usage', error)
    return null
  }
}
