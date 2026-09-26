import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatRate, formatSince, formatVolume, maskIp } from './format'

describe('formatRate', () => {
  it.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])('shows %s bps as 0 kbps', (bps) => {
    expect(formatRate(bps)).toEqual({ value: '0', unit: 'kbps' })
  })

  it.each([
    [500, '0.5', 'kbps'],
    [5_000, '5.0', 'kbps'],
    [50_000, '50', 'kbps'],
    [1_000_000, '1.00', 'Mbps'],
    [25_000_000, '25.0', 'Mbps'],
    [2_500_000_000, '2.50', 'Gbps'],
  ])('shows %s bps as %s %s', (bps, value, unit) => {
    expect(formatRate(bps)).toEqual({ value, unit })
  })

  it.each([
    [999_499, '999', 'kbps'],
    [999_500, '1.00', 'Mbps'],
    [999_999, '1.00', 'Mbps'],
    [999_900_000, '999.9', 'Mbps'],
    [999_960_000, '1.00', 'Gbps'],
  ])('moves to the larger unit when rounding reaches 1000: %s bps is %s %s', (bps, value, unit) => {
    expect(formatRate(bps)).toEqual({ value, unit })
  })

  it.each([
    [9_940, '9.9', 'kbps'],
    [9_960, '10', 'kbps'],
    [9_994_000, '9.99', 'Mbps'],
    [9_996_000, '10.0', 'Mbps'],
  ])('uses the precision of the rounded value: %s bps is %s %s', (bps, value, unit) => {
    expect(formatRate(bps)).toEqual({ value, unit })
  })
})

describe('formatVolume', () => {
  it.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])('shows %s bytes as 0 MB', (bytes) => {
    expect(formatVolume(bytes)).toEqual({ value: '0', unit: 'MB' })
  })

  it.each([
    [512, '512', 'B'],
    [1024, '1.00', 'kB'],
    [1536, '1.50', 'kB'],
    [50 * 1024, '50.0', 'kB'],
    [500 * 1024, '500', 'kB'],
    [5 * 1024 ** 3, '5.00', 'GB'],
    [2 * 1024 ** 4, '2.00', 'TB'],
    [2048 * 1024 ** 4, '2048', 'TB'],
  ])('shows %s bytes in base 1024 as %s %s', (bytes, value, unit) => {
    expect(formatVolume(bytes)).toEqual({ value, unit })
  })

  it.each([
    [1023, '1023', 'B'],
    [1023.5, '1.00', 'kB'],
    [1023 * 1024, '1023', 'kB'],
    [1024 * 1024 - 1, '1.00', 'MB'],
    [1024 ** 3 - 1, '1.00', 'GB'],
    [1023 * 1024 ** 3, '1023', 'GB'],
    [1024 ** 4 - 1, '1.00', 'TB'],
  ])(
    'moves to the larger unit when rounding reaches 1024: %s bytes is %s %s',
    (bytes, value, unit) => {
      expect(formatVolume(bytes)).toEqual({ value, unit })
    },
  )

  it.each([
    [9.994 * 1024, '9.99', 'kB'],
    [9.996 * 1024, '10.0', 'kB'],
    [99.94 * 1024, '99.9', 'kB'],
    [99.96 * 1024, '100', 'kB'],
  ])('uses the precision of the rounded value: %s bytes is %s %s', (bytes, value, unit) => {
    expect(formatVolume(bytes)).toEqual({ value, unit })
  })
})

describe('formatSince', () => {
  const NOW = new Date('2026-09-12T12:00:00Z')
  const ago = (seconds: number) => new Date(NOW.getTime() - seconds * 1000).toISOString()

  afterEach(() => {
    vi.useRealTimers()
  })

  it('says never without a date', () => {
    expect(formatSince()).toBe('nunca')
    expect(formatSince('')).toBe('nunca')
  })

  it.each([
    ['a future date', ago(-30)],
    ['an invalid date', 'not a date'],
    ['under five seconds', ago(4)],
  ])('says now for %s', (_, iso) => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
    expect(formatSince(iso)).toBe('agora')
  })

  it.each([
    [5, 'há 5s'],
    [59, 'há 59s'],
    [60, 'há 1min'],
    [59 * 60, 'há 59min'],
    [60 * 60, 'há 1h'],
    [23 * 60 * 60, 'há 23h'],
    [3 * 24 * 60 * 60, 'há 3d'],
  ])('shows %s seconds ago as "%s"', (seconds, expected) => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
    expect(formatSince(ago(seconds))).toBe(expected)
  })
})

describe('maskIp', () => {
  it.each([
    ['198.51.100.7', '198.51.x.x'],
    ['2001:db8:1:2::7', '2001:db8:…'],
    ['not-an-ip', 'not-an-ip'],
  ])('masks %s as %s', (ip, masked) => {
    expect(maskIp(ip)).toBe(masked)
  })
})
