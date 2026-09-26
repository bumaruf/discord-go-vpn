// Formatters return value and unit apart: the design renders the unit smaller and dimmer.

export interface Measured {
  value: string
  unit: string
}

interface Unit {
  label: string
  decimals: (value: number) => number
}

const RATE_UNITS: Unit[] = [
  { label: 'kbps', decimals: (n) => (n < 10 ? 1 : 0) },
  { label: 'Mbps', decimals: (n) => (n < 10 ? 2 : 1) },
  { label: 'Gbps', decimals: () => 2 },
]

const byteDecimals = (n: number) => (n < 10 ? 2 : n < 100 ? 1 : 0)

const VOLUME_UNITS: Unit[] = [
  { label: 'B', decimals: () => 0 },
  { label: 'kB', decimals: byteDecimals },
  { label: 'MB', decimals: byteDecimals },
  { label: 'GB', decimals: byteDecimals },
  { label: 'TB', decimals: byteDecimals },
]

// Picks the precision from the rounded value, so 9.96 kbps shows as 10 and not 10.0.
function round(n: number, unit: Unit): string {
  return n.toFixed(unit.decimals(Number(n.toFixed(unit.decimals(n)))))
}

function scale(value: number, base: number, units: Unit[]): Measured {
  let i = 0
  let n = value

  // Compares the rounded value, so 999.6 kbps shows as 1.00 Mbps and not 1000 kbps.
  while (Number(round(n, units[i])) >= base && i < units.length - 1) {
    n /= base
    i++
  }

  return { value: round(n, units[i]), unit: units[i].label }
}

export function formatRate(bps: number): Measured {
  if (!Number.isFinite(bps) || bps <= 0) return { value: '0', unit: 'kbps' }
  return scale(bps / 1000, 1000, RATE_UNITS)
}

export function formatVolume(bytes: number): Measured {
  if (!Number.isFinite(bytes) || bytes <= 0) return { value: '0', unit: 'MB' }
  return scale(bytes, 1024, VOLUME_UNITS)
}

export function formatSince(iso?: string): string {
  if (!iso) return 'nunca'

  const seconds = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (!Number.isFinite(seconds) || seconds < 0) return 'agora'
  if (seconds < 5) return 'agora'
  if (seconds < 60) return `há ${seconds}s`

  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `há ${minutes}min`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `há ${hours}h`

  return `há ${Math.floor(hours / 24)}d`
}

/** The mask of `maskOrigin` in internal/provisiond/metrics/peers.go, for the full addresses. */
export function maskIp(ip: string): string {
  const v4 = ip.split('.')
  if (v4.length === 4) return `${v4[0]}.${v4[1]}.x.x`
  const v6 = ip.split(':')
  return v6.length >= 2 ? `${v6[0]}:${v6[1]}:…` : ip
}
