'use client'

import Link from 'next/link'
import { useLiveState } from '@/components/live-provider'
import { BrandMark } from '@/components/ui/brand-mark'
import { StatusDot, type StatusTone } from '@/components/ui/status-dot'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatSince } from '@/lib/format'
import type { LiveSource } from '@/lib/live-snapshot'
import { AccountMenu } from './account-menu'
import { Navigation } from './navigation'
import { ROUTES } from './routes'

const SOURCE_STATUS: Record<LiveSource, { text: string; tone: StatusTone }> = {
  live: { text: 'Ao vivo', tone: 'online' },
  database: { text: 'Sem conexão ao vivo', tone: 'blocked' },
  connecting: { text: 'Conectando', tone: 'offline' },
}

export function Header({ serverName }: { serverName: string }) {
  const { source, snapshot } = useLiveState()
  const status = SOURCE_STATUS[source]

  return (
    <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:px-6">
        <Link
          href={ROUTES.overview.href}
          className="flex min-w-0 items-center gap-2.5 rounded-md text-sm font-medium"
        >
          <BrandMark />
          <span className="hidden sm:inline">Painel VPN</span>
        </Link>
        <span className="hidden text-muted-foreground/40 select-none sm:inline" aria-hidden>
          /
        </span>
        <span className="truncate font-mono text-sm text-muted-foreground">{serverName}</span>

        <div className="ml-auto flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                tabIndex={0}
                className="flex h-7 items-center gap-2 rounded-full border px-2.5 text-xs text-muted-foreground"
              >
                <StatusDot tone={status.tone} pulse={source === 'live'} />
                {status.text}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {snapshot.generated_at
                ? `Atualizado ${formatSince(snapshot.generated_at)}`
                : 'Nenhuma atualização recebida'}
            </TooltipContent>
          </Tooltip>
          <AccountMenu />
        </div>
      </div>

      <div className="mx-auto max-w-6xl px-2 sm:px-4">
        <Navigation peersOnline={snapshot.online} />
      </div>
    </header>
  )
}
