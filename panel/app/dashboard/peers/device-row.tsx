import Link from 'next/link'
import { StatusDot } from '@/components/ui/status-dot'
import { TableCell, TableRow } from '@/components/ui/table'
import { TrafficPair } from '@/components/ui/traffic-pair'
import { formatSince, formatRate, formatVolume } from '@/lib/format'
import { deviceName } from './device-name'
import type { ListedDevice } from './listed-devices'
import { OriginIp } from './origin-ip'

/** The columns that only a machine on the tunnel can fill. */
const OFFLINE = <span className="text-muted-foreground">—</span>

export function DeviceRow({
  device,
  href,
  onOpen,
  invite,
}: {
  device: ListedDevice
  href: string
  onOpen: () => void
  /** Shown next to when it was seen, unless a group header already names the invite. */
  invite?: string
}) {
  const peer = device.peer
  const session = peer && formatVolume(peer.rx_bytes + peer.tx_bytes)

  return (
    <TableRow className="cursor-pointer" onClick={onOpen}>
      <TableCell>
        <div className="flex items-center gap-3">
          <StatusDot tone={peer?.online ? 'online' : 'offline'} />
          <div className="flex min-w-0 flex-col">
            {/* The row's click is for the mouse; the link gives keyboards and new tabs a target. */}
            <Link
              href={href}
              onClick={(event) => event.stopPropagation()}
              className="max-w-56 truncate font-medium hover:underline"
            >
              {deviceName(device)}
            </Link>
            <span className="text-xs text-muted-foreground">
              {peer?.online
                ? formatSince(peer.last_handshake)
                : `visto ${formatSince(device.lastSeenAt)}`}
              {invite ? ` · ${invite}` : null}
            </span>
          </div>
        </div>
      </TableCell>

      <TableCell>
        <OriginIp ip={device.originIp} masked={device.origin} />
      </TableCell>

      <TableCell className="font-mono text-xs text-muted-foreground">
        {peer ? peer.ip : OFFLINE}
      </TableCell>

      <TableCell>
        {peer ? (
          <TrafficPair
            stacked
            down={formatRate(peer.down_bps)}
            up={formatRate(peer.up_bps)}
            className="text-xs"
          />
        ) : (
          OFFLINE
        )}
      </TableCell>

      <TableCell className="tabular-nums">
        {session ? (
          <>
            {session.value} <span className="text-muted-foreground">{session.unit}</span>
          </>
        ) : (
          OFFLINE
        )}
      </TableCell>
    </TableRow>
  )
}
