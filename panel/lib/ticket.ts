import { createHmac } from 'node:crypto'
import { liveSecret } from './env'
import type { Role } from './types'

/** Short because reconnecting the stream is cheap and invisible. */
const TICKET_TTL_SECONDS = 5 * 60

export interface TicketPayload {
  uid: string
  role: Role
  exp: number
}

/**
 * Signs the read-only pass the browser uses to open the live stream on provisiond; actions still
 * need VPN_PANEL_SECRET, which never leaves the server. The format, `<payload>.<signature>` in
 * unpadded base64url, must match `ValidateTicketAt` in internal/provisiond/auth/ticket.go.
 */
export function issueTicket(uid: string, role: Role): string {
  const secret = liveSecret()
  if (!secret) {
    throw new Error('set VPN_LIVE_SECRET (the same value as on provisiond)')
  }

  const payload: TicketPayload = {
    uid,
    role,
    exp: Math.floor(Date.now() / 1000) + TICKET_TTL_SECONDS,
  }

  const body = Buffer.from(JSON.stringify(payload)).toString('base64url')
  const signature = createHmac('sha256', secret).update(body).digest('base64url')

  return `${body}.${signature}`
}
