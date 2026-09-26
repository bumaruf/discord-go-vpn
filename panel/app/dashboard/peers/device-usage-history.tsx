'use client'

import { useEffect, useState } from 'react'
import { ChartNotice, DailyBars, NoUsageNotice, UsageMetrics } from '@/components/charts/daily-bars'
import { Skeleton } from '@/components/ui/skeleton'
import type { UsageSummary } from '@/lib/usage'

type HistoryState =
  | { phase: 'loading' }
  | { phase: 'ready'; summary: UsageSummary }
  | { phase: 'error'; message: string }

/**
 * Fetched once, not per stream tick: daily usage changes every 30s at best. The parent remounts it
 * with `key` per device, so it starts `loading` instead of flashing the previous device's data.
 */
export function DeviceUsageHistory({ deviceId }: { deviceId?: string }) {
  const [state, setState] = useState<HistoryState>({ phase: 'loading' })

  useEffect(() => {
    if (!deviceId) return

    const controller = new AbortController()

    async function load() {
      try {
        const r = await fetch(`/api/usage?device=${encodeURIComponent(deviceId!)}`, {
          signal: controller.signal,
        })
        if (!r.ok) {
          setState({
            phase: 'error',
            message:
              r.status === 503
                ? 'O banco não respondeu, então o histórico está indisponível.'
                : 'O histórico não carregou. Recarregue a página para tentar de novo.',
          })
          return
        }
        setState({ phase: 'ready', summary: (await r.json()) as UsageSummary })
      } catch {
        if (!controller.signal.aborted) {
          setState({
            phase: 'error',
            message: 'O histórico não carregou. Recarregue a página para tentar de novo.',
          })
        }
      }
    }

    void load()
    return () => controller.abort()
  }, [deviceId])

  if (!deviceId) {
    return <ChartNotice message="Esta máquina não foi registrada no banco e não tem histórico." />
  }

  if (state.phase === 'loading') {
    return <Skeleton className="h-60 w-full" />
  }
  if (state.phase === 'error') {
    return <ChartNotice message={state.message} />
  }

  const isEmpty = state.summary.totalRx + state.summary.totalTx === 0
  if (isEmpty) {
    return <NoUsageNotice />
  }

  return (
    <div className="flex flex-col gap-6">
      <UsageMetrics summary={state.summary} />
      <DailyBars summary={state.summary} />
    </div>
  )
}
