// Kept apart from lib/invites.ts, which reaches Mongo, so client components can import it.

/** The highest simultaneous-lease cap the panel accepts. */
export const MAX_LEASES_LIMIT = 100

/**
 * Reads the cap typed in a form: `null` for an empty field, meaning no cap, and `undefined` for
 * anything but a whole number from 1 to MAX_LEASES_LIMIT.
 */
export function parseMaxLeases(raw: unknown): number | null | undefined {
  const text = typeof raw === 'string' ? raw.trim() : ''
  if (text === '') return null
  if (!/^\d+$/.test(text)) return undefined

  const value = Number(text)
  return value >= 1 && value <= MAX_LEASES_LIMIT ? value : undefined
}
