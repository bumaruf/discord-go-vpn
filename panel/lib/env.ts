// Every read is a function so values are taken at request time: `next build` evaluates modules
// without runtime env, and tests stub env per case. Next inlines NEXT_PUBLIC_* only where the full
// literal `process.env.NAME` appears, so each variable keeps its own literal reference.

const DEFAULT_MONGODB_DB = 'vpn'
const DEFAULT_SERVER_NAME = 'wg0'
const DEFAULT_WG_PORT = '51820/udp'

// Vercel and .env.example leave unused variables empty; an empty or blank value counts as unset.
function present(value: string | undefined): string | undefined {
  return value?.trim() ? value : undefined
}

export function adminUsername(): string | undefined {
  return present(process.env.ADMIN_USERNAME)
}

export function adminPasswordHash(): string | undefined {
  return present(process.env.ADMIN_PASSWORD_HASH)
}

export function adminPassword(): string | undefined {
  return present(process.env.ADMIN_PASSWORD)
}

export function mongodbUri(): string | undefined {
  return present(process.env.MONGODB_URI)
}

export function mongodbDb(): string {
  return present(process.env.MONGODB_DB) ?? DEFAULT_MONGODB_DB
}

export function provisiondBaseUrl(): string | undefined {
  return present(process.env.PROVISIOND_URL)
}

export function panelSecret(): string | undefined {
  return present(process.env.VPN_PANEL_SECRET)
}

export function liveSecret(): string | undefined {
  return present(process.env.VPN_LIVE_SECRET)
}

/** The address clients lease from, which goes into the connection token; not PROVISIOND_URL. */
export function provisiondLeaseUrl(): string | undefined {
  return present(process.env.PROVISIOND_LEASE_URL)
}

export function provisiondFingerprint(): string | undefined {
  return present(process.env.PROVISIOND_FINGERPRINT)
}

/** `null` without a positive quota: the usage KPI then shows no progress bar. */
export function monthlyQuotaGb(): number | null {
  const raw = present(process.env.MONTHLY_QUOTA_GB)
  if (!raw) return null

  const gb = Number(raw)
  return Number.isFinite(gb) && gb > 0 ? gb : null
}

export function serverName(): string {
  const name = present(process.env.NEXT_PUBLIC_SERVER_NAME)
  return name?.trim() ?? DEFAULT_SERVER_NAME
}

export function wireguardPort(): string {
  const port = present(process.env.NEXT_PUBLIC_WG_PORT)
  return port?.trim() ?? DEFAULT_WG_PORT
}
