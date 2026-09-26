import { NextResponse } from 'next/server'
import { errorResponse } from '@/lib/api-response'
import { EMPTY_SNAPSHOT, type LiveSnapshot } from '@/lib/live-snapshot'
import { peerFromSession } from '@/lib/live-fallback'
import { findLatestServerSample } from '@/lib/samples'
import { findSessionsUpdatedSince } from '@/lib/sessions'
import { deniedResponse, requireAdmin } from '@/lib/require-admin'

export const runtime = 'nodejs'
export const dynamic = 'force-dynamic'

/**
 * Fallback while the stream is down, never the normal path: polling it would cost an invocation
 * every few seconds and serve older data through Atlas.
 */
export async function GET() {
  const admin = await requireAdmin()
  if (!admin.ok) return deniedResponse(admin.status)

  try {
    // A session not updated for 2 minutes is a dead lease the collector never closed.
    const cutoff = new Date(Date.now() - 2 * 60 * 1000)
    const sessions = await findSessionsUpdatedSince(cutoff)
    const latest = await findLatestServerSample()

    const peers = sessions.map(peerFromSession)

    const snapshot: LiveSnapshot = {
      ...EMPTY_SNAPSHOT,
      generated_at: (latest?.at ?? new Date()).toISOString(),
      down_bps: latest?.downBps ?? 0,
      up_bps: latest?.upBps ?? 0,
      online: peers.filter((p) => p.online).length,
      active: peers.length,
      peers,
    }

    return NextResponse.json(snapshot, { headers: { 'Cache-Control': 'no-store' } })
  } catch (error) {
    console.error('failed to read the database', error)
    return errorResponse('banco indisponível', 503)
  }
}
