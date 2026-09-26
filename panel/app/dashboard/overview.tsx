'use client'

import { ArrowDown, ArrowUp, HardDrive, MonitorSmartphone, type LucideIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { ChartNotice, DailyBars, NoUsageNotice, UsageTotals } from '@/components/charts/daily-bars'
import { Throughput } from '@/components/charts/throughput'
import { TrafficLegend } from '@/components/charts/traffic-series'
import { useLiveState } from '@/components/live-provider'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { UsageFromDatabase } from '@/lib/usage'
import { USAGE_DAYS } from '@/lib/usage-days'
import { formatRate, formatVolume } from '@/lib/format'
import { cn } from '@/lib/utils'

const GB = 1024 ** 3

type TrafficRange = 'minute' | 'month'

export function Overview({
  month,
  history,
  quotaGb,
}: {
  month: UsageFromDatabase
  history: UsageFromDatabase
  quotaGb: number | null
}) {
  const { snapshot, history: liveHistory, source } = useLiveState()
  const [range, setRange] = useState<TrafficRange>('minute')

  const isLive = source === 'live'
  const down = formatRate(snapshot.down_bps)
  const up = formatRate(snapshot.up_bps)

  const monthTotal = month ? month.totalRx + month.totalTx : 0
  const monthVolume = formatVolume(monthTotal)
  const quotaUsage = quotaGb ? Math.min((monthTotal / GB / quotaGb) * 100, 100) : null

  const hasHistory = history !== null && history.totalRx + history.totalTx > 0

  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi
          label="Máquinas conectadas"
          icon={MonitorSmartphone}
          value={`${snapshot.online}`}
          unit={`de ${snapshot.active}`}
          footer={
            snapshot.limit > 0
              ? `Limite de ${snapshot.limit} simultâneas`
              : 'Limite não informado pelo servidor'
          }
        />
        <Kpi
          label="Download agora"
          icon={ArrowDown}
          iconClassName="text-download"
          value={isLive ? down.value : '—'}
          unit={isLive ? down.unit : undefined}
          footer={isLive ? 'Amostra do último segundo' : 'Sem conexão ao vivo'}
        />
        <Kpi
          label="Upload agora"
          icon={ArrowUp}
          iconClassName="text-upload"
          value={isLive ? up.value : '—'}
          unit={isLive ? up.unit : undefined}
          footer={isLive ? 'Amostra do último segundo' : 'Sem conexão ao vivo'}
        />
        <Kpi
          label="Consumo do mês"
          icon={HardDrive}
          value={month ? monthVolume.value : '—'}
          unit={month ? monthVolume.unit : undefined}
          footer={
            !month
              ? 'O banco não respondeu'
              : quotaUsage !== null
                ? `${quotaUsage.toFixed(0)}% da cota de ${quotaGb} GB`
                : 'Desde o dia 1º'
          }
          progress={month ? quotaUsage : null}
        />
      </div>

      <Card>
        <Tabs
          value={range}
          onValueChange={(value) => setRange(value as TrafficRange)}
          className="gap-(--card-spacing)"
        >
          <ChartHeader
            title="Utilização"
            description={
              range === 'minute'
                ? 'Todas as máquinas somadas, ao vivo no último minuto'
                : 'Todas as máquinas, do dia 1º até hoje'
            }
            aside={range === 'minute' && isLive ? <TrafficLegend /> : null}
            action={
              <TabsList>
                <TabsTrigger value="minute">Último minuto</TabsTrigger>
                <TabsTrigger value="month">Este mês</TabsTrigger>
              </TabsList>
            }
          />
          <CardContent>
            <TabsContent value="minute">
              {isLive ? (
                <Throughput history={liveHistory} />
              ) : (
                <ChartNotice message="O gráfico ao vivo volta quando o painel reconectar ao servidor." />
              )}
            </TabsContent>
            <TabsContent value="month">
              {month === null ? (
                <ChartNotice message="O banco não respondeu, então o consumo do mês está indisponível." />
              ) : monthTotal > 0 ? (
                <DailyBars summary={month} />
              ) : (
                <ChartNotice message="Nenhum consumo registrado neste mês." />
              )}
            </TabsContent>
          </CardContent>
        </Tabs>
      </Card>

      <Card>
        <ChartHeader
          title="Consumo diário"
          description={`Todas as máquinas, nos últimos ${USAGE_DAYS} dias`}
          aside={hasHistory ? <UsageTotals summary={history} /> : null}
        />
        <CardContent>
          {history === null ? (
            <ChartNotice message="O banco não respondeu, então o histórico está indisponível." />
          ) : hasHistory ? (
            <DailyBars summary={history} />
          ) : (
            <NoUsageNotice />
          )}
        </CardContent>
      </Card>
    </>
  )
}

/** The aside (legend or totals) is hidden on phones, where it would squeeze the description. */
function ChartHeader({
  title,
  description,
  aside,
  action,
}: {
  title: string
  description: string
  aside: ReactNode
  action?: ReactNode
}) {
  return (
    <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
      <div className="flex flex-col gap-1">
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </div>
      {aside || action ? (
        <div className="flex shrink-0 items-center gap-4">
          {aside ? <div className="hidden sm:flex">{aside}</div> : null}
          {action}
        </div>
      ) : null}
    </CardHeader>
  )
}

function Kpi({
  label,
  icon: Icon,
  iconClassName,
  value,
  unit,
  footer,
  progress,
}: {
  label: string
  icon: LucideIcon
  iconClassName?: string
  value: string
  unit?: string
  footer: string
  progress?: number | null
}) {
  return (
    <Card className="gap-3">
      <CardHeader className="flex items-center justify-between gap-2">
        <CardDescription>{label}</CardDescription>
        <Icon className={cn('size-4 text-muted-foreground', iconClassName)} aria-hidden />
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <p className="text-3xl font-semibold tracking-tight whitespace-nowrap tabular-nums">
          {value}
          {unit ? (
            <span className="ml-1.5 text-base font-normal tracking-normal text-muted-foreground">
              {unit}
            </span>
          ) : null}
        </p>
        {progress !== null && progress !== undefined ? (
          <Progress value={progress} aria-label={footer} />
        ) : null}
        <p className="text-xs text-muted-foreground">{footer}</p>
      </CardContent>
    </Card>
  )
}
