import type { LivePeer } from '@/lib/live-snapshot'

export function countOnlineByInvite(peers: LivePeer[]): Map<string, number> {
  const onlineByInvite = new Map<string, number>()
  for (const p of peers) {
    if (!p.online || !p.invite_id) continue
    onlineByInvite.set(p.invite_id, (onlineByInvite.get(p.invite_id) ?? 0) + 1)
  }
  return onlineByInvite
}
