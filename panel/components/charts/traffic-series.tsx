import type { ChartConfig } from '@/components/ui/chart'
import type { Measured } from '@/lib/format'
import { cn } from '@/lib/utils'

export const TRAFFIC_SERIES = {
  download: { label: 'Download', color: 'var(--chart-1)' },
  upload: { label: 'Upload', color: 'var(--chart-2)' },
} satisfies ChartConfig

type TrafficKey = keyof typeof TRAFFIC_SERIES

interface TooltipEntry {
  dataKey?: unknown
  value?: unknown
  payload?: unknown
}

export function TrafficTooltip<Datum>({
  active,
  payload,
  title,
  format,
}: {
  active?: boolean
  payload?: ReadonlyArray<TooltipEntry>
  title: (datum: Datum) => string
  format: (value: number) => Measured
}) {
  if (!active || !payload?.length) return null

  return (
    <div className="grid min-w-44 gap-1.5 rounded-lg border bg-popover px-3 py-2 text-xs shadow-xl">
      <p className="font-medium">{title(payload[0].payload as Datum)}</p>
      {payload.map((entry) => {
        const key = String(entry.dataKey) as TrafficKey
        const measured = format(Number(entry.value))

        return (
          <div key={key} className="flex items-center gap-2">
            <span
              className="size-2 rounded-[2px]"
              style={{ background: TRAFFIC_SERIES[key].color }}
            />
            <span className="text-muted-foreground">{TRAFFIC_SERIES[key].label}</span>
            <span className="ml-auto pl-4 font-medium tabular-nums">
              {measured.value} <span className="text-muted-foreground">{measured.unit}</span>
            </span>
          </div>
        )
      })}
    </div>
  )
}

export function TrafficLegend({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-4 text-xs text-muted-foreground', className)}>
      {Object.entries(TRAFFIC_SERIES).map(([key, series]) => (
        <span key={key} className="flex items-center gap-1.5">
          <span className="size-2 rounded-[2px]" style={{ background: series.color }} />
          {series.label}
        </span>
      ))}
    </div>
  )
}
