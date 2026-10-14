import { readProvisiond } from './provisiond'
import { USAGE_DAYS } from './usage-days'

// Sums the per-device deltas the collector stores, not WireGuard counters, which reset on every
// lease; that is what lets a device's history span its leases.

/** GET /admin/usage (testdata/contracts/panel-usage.json): days without traffic are absent. */
export interface UsageResponse {
  days: { day: string; rx_bytes: number; tx_bytes: number }[]
}

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

export interface ServerUsage {
  /** From the 1st of the current UTC month to today. */
  month: UsageFromDatabase
  /** The last USAGE_DAYS days. */
  history: UsageFromDatabase
}

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

async function usageSince(from: string, device?: string): Promise<DailyUsage[]> {
  const query = new URLSearchParams({ from, ...(device ? { device } : {}) })
  const { days } = await readProvisiond<UsageResponse>(`/admin/usage?${query}`)
  return days.map((d) => ({ day: d.day, rxBytes: d.rx_bytes, txBytes: d.tx_bytes }))
}

/** `identity` is the device id. */
export async function findDeviceUsage(identity: string): Promise<UsageFromDatabase> {
  try {
    const days = lastDays()
    return summarize(days, await usageSince(days[0], identity))
  } catch (error) {
    console.error('failed to read device usage', error)
    return null
  }
}

/** One request covers both ranges: the month and the history overlap. */
export async function findServerUsage(): Promise<ServerUsage> {
  const month = monthDays()
  const history = lastDays()
  try {
    const rows = await usageSince(month[0] < history[0] ? month[0] : history[0])
    return { month: summarize(month, rows), history: summarize(history, rows) }
  } catch (error) {
    console.error('failed to read server usage', error)
    return { month: null, history: null }
  }
}
