import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  adminPassword,
  adminPasswordHash,
  adminUsername,
  liveSecret,
  mongodbDb,
  mongodbUri,
  monthlyQuotaGb,
  panelSecret,
  provisiondBaseUrl,
  provisiondFingerprint,
  provisiondLeaseUrl,
  serverName,
  wireguardPort,
} from './env'

afterEach(() => {
  vi.unstubAllEnvs()
})

describe.each([
  ['ADMIN_USERNAME', adminUsername],
  ['ADMIN_PASSWORD_HASH', adminPasswordHash],
  ['ADMIN_PASSWORD', adminPassword],
  ['MONGODB_URI', mongodbUri],
  ['PROVISIOND_URL', provisiondBaseUrl],
  ['PROVISIOND_LEASE_URL', provisiondLeaseUrl],
  ['PROVISIOND_FINGERPRINT', provisiondFingerprint],
  ['VPN_PANEL_SECRET', panelSecret],
  ['VPN_LIVE_SECRET', liveSecret],
])('%s', (name, read) => {
  it('is undefined when unset', () => {
    vi.stubEnv(name, undefined)
    expect(read()).toBeUndefined()
  })

  it.each(['', '   '])('is undefined when set to %j', (value) => {
    vi.stubEnv(name, value)
    expect(read()).toBeUndefined()
  })

  it('returns the configured value unchanged', () => {
    vi.stubEnv(name, ' value ')
    expect(read()).toBe(' value ')
  })
})

describe('mongodbDb', () => {
  it.each([undefined, '', '   '])('defaults to vpn when MONGODB_DB is %j', (value) => {
    vi.stubEnv('MONGODB_DB', value)
    expect(mongodbDb()).toBe('vpn')
  })

  it('uses the configured database', () => {
    vi.stubEnv('MONGODB_DB', 'vpn-staging')
    expect(mongodbDb()).toBe('vpn-staging')
  })
})

describe('monthlyQuotaGb', () => {
  it.each([undefined, '', '   ', '0', '-5', 'abc'])(
    'is null when MONTHLY_QUOTA_GB is %j',
    (value) => {
      vi.stubEnv('MONTHLY_QUOTA_GB', value)
      expect(monthlyQuotaGb()).toBeNull()
    },
  )

  it('parses a positive quota', () => {
    vi.stubEnv('MONTHLY_QUOTA_GB', '3000')
    expect(monthlyQuotaGb()).toBe(3000)
  })
})

describe('serverName', () => {
  it.each([undefined, '', '   '])('defaults to wg0 when NEXT_PUBLIC_SERVER_NAME is %j', (value) => {
    vi.stubEnv('NEXT_PUBLIC_SERVER_NAME', value)
    expect(serverName()).toBe('wg0')
  })

  it('uses the configured name, without surrounding whitespace', () => {
    vi.stubEnv('NEXT_PUBLIC_SERVER_NAME', ' vpn-sp ')
    expect(serverName()).toBe('vpn-sp')
  })
})

describe('wireguardPort', () => {
  it.each([undefined, '', '   '])(
    'defaults to 51820/udp when NEXT_PUBLIC_WG_PORT is %j',
    (value) => {
      vi.stubEnv('NEXT_PUBLIC_WG_PORT', value)
      expect(wireguardPort()).toBe('51820/udp')
    },
  )

  it('uses the configured port, without surrounding whitespace', () => {
    vi.stubEnv('NEXT_PUBLIC_WG_PORT', ' 443/udp ')
    expect(wireguardPort()).toBe('443/udp')
  })
})
