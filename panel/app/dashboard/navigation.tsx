'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { isRouteActive, PANEL_ROUTES, ROUTES } from './routes'

export function Navigation({ peersOnline }: { peersOnline: number }) {
  const pathname = usePathname()

  return (
    <nav className="-mb-px flex overflow-x-auto" aria-label="Seções do painel">
      {PANEL_ROUTES.map((route) => {
        const active = isRouteActive(route.href, pathname)

        return (
          <Link
            key={route.href}
            href={route.href}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'group relative flex h-11 shrink-0 items-center px-1 text-sm text-muted-foreground transition-colors hover:text-foreground',
              'after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:rounded-full after:bg-foreground after:opacity-0',
              active && 'text-foreground after:opacity-100',
            )}
          >
            <span className="flex items-center gap-2 rounded-md px-2 py-1.5 group-hover:bg-accent">
              {route.title}
              {route === ROUTES.peers && peersOnline > 0 ? (
                <Badge variant="secondary" className="tabular-nums">
                  {peersOnline}
                </Badge>
              ) : null}
            </span>
          </Link>
        )
      })}
    </nav>
  )
}
