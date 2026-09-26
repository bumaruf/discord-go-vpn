import { ArrowDown, ArrowUp } from 'lucide-react'
import type { Measured } from '@/lib/format'
import { cn } from '@/lib/utils'

export function TrafficPair({
  down,
  up,
  stacked = false,
  className,
}: {
  down: Measured
  up: Measured
  stacked?: boolean
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-flex tabular-nums',
        stacked ? 'flex-col gap-0.5' : 'items-center gap-3',
        className,
      )}
    >
      <Reading measured={down} label="download">
        <ArrowDown className="size-3.5 text-download" aria-hidden />
      </Reading>
      <Reading measured={up} label="upload">
        <ArrowUp className="size-3.5 text-upload" aria-hidden />
      </Reading>
    </span>
  )
}

function Reading({
  measured,
  label,
  children,
}: {
  measured: Measured
  label: string
  children: React.ReactNode
}) {
  return (
    <span className="inline-flex items-center gap-1 whitespace-nowrap">
      {children}
      <span className="sr-only">{label}</span>
      {measured.value}
      <span className="text-muted-foreground">{measured.unit}</span>
    </span>
  )
}
