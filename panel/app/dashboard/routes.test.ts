import { describe, expect, it } from 'vitest'
import { isRouteActive } from './routes'

describe('isRouteActive', () => {
  it('marks the overview only on its exact path', () => {
    expect(isRouteActive('/dashboard', '/dashboard')).toBe(true)
    expect(isRouteActive('/dashboard', '/dashboard/peers')).toBe(false)
  })

  it('marks another page on its path and below', () => {
    expect(isRouteActive('/dashboard/peers', '/dashboard/peers')).toBe(true)
    expect(isRouteActive('/dashboard/peers', '/dashboard/peers/detail')).toBe(true)
    expect(isRouteActive('/dashboard/peers', '/dashboard/invites')).toBe(false)
  })
})
