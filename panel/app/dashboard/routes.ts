export const ROUTES = {
  overview: { href: '/dashboard', title: 'Visão geral' },
  peers: { href: '/dashboard/peers', title: 'Quem está usando' },
  invites: { href: '/dashboard/invites', title: 'Convites' },
} as const

export type PanelRoute = (typeof ROUTES)[keyof typeof ROUTES]

export const PANEL_ROUTES: PanelRoute[] = [ROUTES.overview, ROUTES.peers, ROUTES.invites]

// The overview's path prefixes every other page, so it is active only on an exact match.
export function isRouteActive(href: string, pathname: string): boolean {
  return href === ROUTES.overview.href ? pathname === href : pathname.startsWith(href)
}
