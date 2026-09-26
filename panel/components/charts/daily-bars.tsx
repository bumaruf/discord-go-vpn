'use client'

import { Activity } from 'lucide-react'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia } from '@/components/ui/empty'
import { TrafficPair } from '@/components/ui/traffic-pair'
import type { UsageSummary } from '@/lib/usage'
import { USAGE_DAYS } from '@/lib/usage-days'
import { formatVolume, type Measured } from '@/lib/format'
import { TRAFFIC_SERIES, TrafficLegend, TrafficTooltip } from './traffic-series'

interface DayUsage {
  day: string
  download: number
  upload: number
}

const shortDate = new Intl.DateTimeFormat('pt-BR', { day: '2-digit', month: '2-digit' })
const longDate = new Intl.DateTimeFormat('pt-BR', {
  weekday: 'short',
  day: 'numeric',
  month: 'long',
})

// A bare date parses as UTC midnight, which Brazilian time zones show as the day before.
function parseDay(day: string): Date {
  return new Date(`${day}T12:00:00`)
}

function formatVolumeTick(bytes: number): string {
  if (bytes === 0) return '0'
  const volume = formatVolume(bytes)
  return `${volume.value} ${volume.unit}`
}

export function DailyBars({ summary }: { summary: UsageSummary }) {
  // The server's tx is the user's download; swapped, the chart would still look plausible.
  const data: DayUsage[] = summary.days.map((d) => ({
    day: d.day,
    download: d.txBytes,
    upload: d.rxBytes,
  }))

  return (
    <div className="flex flex-col gap-4">
      <ChartContainer config={TRAFFIC_SERIES} className="aspect-auto h-60 w-full">
        <BarChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid vertical={false} strokeDasharray="3 3" />
          <XAxis
            dataKey="day"
            tickFormatter={(day: string) => shortDate.format(parseDay(day))}
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={12}
          />
          <YAxis
            width={80}
            tickFormatter={formatVolumeTick}
            tickLine={false}
            axisLine={false}
            tickCount={4}
          />
          <ChartTooltip
            cursor={{ fill: 'var(--muted)', fillOpacity: 0.6 }}
            isAnimationActive={false}
            content={(props) => (
              <TrafficTooltip<DayUsage>
                active={props.active}
                payload={props.payload}
                format={formatVolume}
                title={(usage) => longDate.format(parseDay(usage.day))}
              />
            )}
          />
          <Bar
            dataKey="download"
            stackId="traffic"
            fill="var(--color-download)"
            maxBarSize={36}
            isAnimationActive={false}
          />
          <Bar
            dataKey="upload"
            stackId="traffic"
            fill="var(--color-upload)"
            radius={[4, 4, 0, 0]}
            maxBarSize={36}
            isAnimationActive={false}
          />
        </BarChart>
      </ChartContainer>
      <TrafficLegend />
    </div>
  )
}

export function UsageTotals({ summary }: { summary: UsageSummary }) {
  return (
    <span className="flex items-center gap-3 text-sm">
      <span className="text-muted-foreground">Total</span>
      <TrafficPair down={formatVolume(summary.totalTx)} up={formatVolume(summary.totalRx)} />
    </span>
  )
}

export function UsageMetrics({ summary }: { summary: UsageSummary }) {
  const total = summary.totalRx + summary.totalTx
  const daysWithTraffic = summary.days.filter((d) => d.rxBytes + d.txBytes > 0).length

  return (
    <dl className="grid gap-4 sm:grid-cols-3">
      <Metric label={`Total em ${summary.days.length} dias`} measured={formatVolume(total)} />
      <Metric
        label="Média por dia com uso"
        measured={formatVolume(daysWithTraffic === 0 ? 0 : total / daysWithTraffic)}
      />
      <Metric
        label="Dias com uso"
        measured={{ value: String(daysWithTraffic), unit: `de ${summary.days.length}` }}
      />
    </dl>
  )
}

function Metric({ label, measured }: { label: string; measured: Measured }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-2xl font-semibold tracking-tight tabular-nums">
        {measured.value}
        <span className="ml-1.5 text-sm font-normal tracking-normal text-muted-foreground">
          {measured.unit}
        </span>
      </dd>
    </div>
  )
}

export function ChartNotice({ message }: { message: string }) {
  return (
    <Empty className="h-60 border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Activity />
        </EmptyMedia>
        <EmptyDescription>{message}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

export function NoUsageNotice() {
  return <ChartNotice message={`Nenhum consumo registrado nos últimos ${USAGE_DAYS} dias.`} />
}
