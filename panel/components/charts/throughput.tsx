'use client'

import { useMemo } from 'react'
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import { TrafficPair } from '@/components/ui/traffic-pair'
import { WINDOW_SIZE, type ThroughputPoint } from '@/hooks/use-live'
import { formatRate } from '@/lib/format'
import { TRAFFIC_SERIES, TrafficTooltip } from './traffic-series'

interface Sample {
  /** Seconds relative to the latest sample: 0 is now, negative is the past. */
  t: number
  download: number
  upload: number
}

const X_TICKS = [-45, -30, -15, 0]

function formatTick(t: number): string {
  return t === 0 ? 'agora' : `${t}s`
}

function formatRateTick(bps: number): string {
  if (bps === 0) return '0'
  const rate = formatRate(bps)
  return `${rate.value} ${rate.unit}`
}

export function Throughput({ history }: { history: ThroughputPoint[] }) {
  const { data, now, average, peak } = useMemo(() => {
    const last = history.length - 1
    const data: Sample[] = history.map((p, i) => ({ t: i - last, download: p.down, upload: p.up }))

    const down = history.map((p) => p.down)
    const up = history.map((p) => p.up)
    const mean = (values: number[]) =>
      values.length ? values.reduce((a, b) => a + b, 0) / values.length : 0

    return {
      data,
      now: { down: history.at(-1)?.down ?? 0, up: history.at(-1)?.up ?? 0 },
      average: { down: mean(down), up: mean(up) },
      peak: { down: Math.max(...down, 0), up: Math.max(...up, 0) },
    }
  }, [history])

  return (
    <div className="flex flex-col gap-6">
      <ChartContainer config={TRAFFIC_SERIES} className="aspect-auto h-60 w-full">
        <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <defs>
            {Object.keys(TRAFFIC_SERIES).map((key) => (
              <linearGradient key={key} id={`throughput-${key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={`var(--color-${key})`} stopOpacity={0.3} />
                <stop offset="100%" stopColor={`var(--color-${key})`} stopOpacity={0} />
              </linearGradient>
            ))}
          </defs>
          <CartesianGrid vertical={false} strokeDasharray="3 3" />
          {/* A fixed one-minute domain, so the line fills from the right instead of stretching. */}
          <XAxis
            dataKey="t"
            type="number"
            domain={[-(WINDOW_SIZE - 1), 0]}
            ticks={X_TICKS}
            tickFormatter={formatTick}
            tickLine={false}
            axisLine={false}
            tickMargin={8}
          />
          <YAxis
            width={80}
            tickFormatter={formatRateTick}
            tickLine={false}
            axisLine={false}
            tickCount={4}
          />
          <ChartTooltip
            cursor={{ stroke: 'var(--input)' }}
            isAnimationActive={false}
            content={(props) => (
              <TrafficTooltip<Sample>
                active={props.active}
                payload={props.payload}
                format={formatRate}
                title={(sample) => (sample.t === 0 ? 'Agora' : `Há ${-sample.t}s`)}
              />
            )}
          />
          {Object.keys(TRAFFIC_SERIES).map((key) => (
            <Area
              key={key}
              dataKey={key}
              type="monotone"
              stroke={`var(--color-${key})`}
              strokeWidth={1.5}
              fill={`url(#throughput-${key})`}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </AreaChart>
      </ChartContainer>

      <div className="grid gap-4 border-t pt-4 sm:grid-cols-3">
        <Reading label="Agora" down={now.down} up={now.up} />
        <Reading label="Média em 1 min" down={average.down} up={average.up} />
        <Reading label="Pico em 1 min" down={peak.down} up={peak.up} />
      </div>
    </div>
  )
}

function Reading({ label, down, up }: { label: string; down: number; up: number }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <TrafficPair down={formatRate(down)} up={formatRate(up)} className="text-sm" />
    </div>
  )
}
