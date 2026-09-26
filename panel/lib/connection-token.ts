import { provisiondFingerprint, provisiondLeaseUrl } from './env'

// The client decodes this in Go; testdata/contracts/connection-token.json is the payload both sides check.
const PREFIX = 'dgv1_'

export interface ConnectionTokenPayload {
  server: string
  fingerprint: string
  invite: string
}

export function buildConnectionToken({
  server,
  fingerprint,
  invite,
}: ConnectionTokenPayload): string {
  const json = JSON.stringify({ server, fingerprint, invite })
  return PREFIX + Buffer.from(json, 'utf8').toString('base64url')
}

/** `undefined` while PROVISIOND_LEASE_URL or PROVISIOND_FINGERPRINT is unset. */
export function connectionTokenFor(invite: string): string | undefined {
  const server = provisiondLeaseUrl()
  const fingerprint = provisiondFingerprint()
  if (!server || !fingerprint) return undefined

  // The client appends /lease to server, so a trailing slash would double it.
  const base = server.trim().replace(/\/+$/, '')
  return buildConnectionToken({ server: base, fingerprint: fingerprint.trim(), invite })
}
