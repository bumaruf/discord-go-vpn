import { ChevronLeft } from 'lucide-react'
import Link from 'next/link'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { StatusDot } from '@/components/ui/status-dot'
import type { ReactNode } from 'react'
import { formatSince, formatVolume } from '@/lib/format'
import { USAGE_DAYS } from '@/lib/usage-days'
import { cn } from '@/lib/utils'
import { ROUTES } from '../routes'
import { DeviceActions } from './device-actions'
import { deviceName } from './device-name'
import { DeviceUsageHistory } from './device-usage-history'
import type { ListedDevice } from './listed-devices'
import { OriginIp } from './origin-ip'

export function DeviceDetail({
  device,
  inviteLabel,
  backHref,
}: {
  device: ListedDevice
  inviteLabel: string
  /** The list with the filters it had when the machine was opened. */
  backHref: string
}) {
  const peer = device.peer
  const rx = peer && formatVolume(peer.rx_bytes)
  const tx = peer && formatVolume(peer.tx_bytes)

  return (
    <>
      <PageHeader
        back={
          <Link
            href={backHref}
            className="mb-2 inline-flex w-fit items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <ChevronLeft className="size-4" aria-hidden />
            {ROUTES.peers.title}
          </Link>
        }
        title={deviceName(device)}
        description={
          <span className="inline-flex items-center gap-2">
            <StatusDot tone={peer?.online ? 'online' : 'offline'} />
            {peer?.online
              ? `Conectada, último handshake ${formatSince(peer.last_handshake)}`
              : `Desconectada, vista ${formatSince(device.lastSeenAt)}`}
          </span>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Conexão</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-x-6 gap-y-5 sm:grid-cols-2 lg:grid-cols-4">
            <DetailItem label="Entrou pelo convite" value={inviteLabel} />
            <DetailItem
              label="Origem"
              value={<OriginIp ip={device.originIp} masked={device.origin} />}
            />
            <DetailItem label="Primeiro acesso" value={formatSince(device.firstSeenAt)} />
            <DetailItem label="Conexões" value={device.accessCount?.toString() ?? '—'} />

            {peer ? (
              <>
                <DetailItem label="IP interno" value={peer.ip} mono />
                <DetailItem label="Recebido do peer" value={`${rx!.value} ${rx!.unit}`} />
                <DetailItem label="Enviado ao peer" value={`${tx!.value} ${tx!.unit}`} />
                <DetailItem label="Chave pública" value={peer.key} mono wide />
              </>
            ) : null}
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Consumo diário</CardTitle>
          <CardDescription>Esta máquina, nos últimos {USAGE_DAYS} dias</CardDescription>
        </CardHeader>
        <CardContent>
          <DeviceUsageHistory key={device.id} deviceId={device.deviceId} />
        </CardContent>
      </Card>

      <DeviceActions device={device} />
    </>
  )
}

function DetailItem({
  label,
  value,
  mono,
  wide,
}: {
  label: string
  value: ReactNode
  mono?: boolean
  wide?: boolean
}) {
  return (
    <div className={cn('flex min-w-0 flex-col gap-1', wide && 'sm:col-span-2 lg:col-span-4')}>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={cn('text-sm break-all', mono && 'font-mono text-xs leading-5')}>{value}</dd>
    </div>
  )
}
