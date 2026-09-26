import { unstable_doesMiddlewareMatch } from 'next/experimental/testing/server'
import { describe, expect, it, vi } from 'vitest'
import nextConfig from './next.config'
import { config } from './proxy'

// next-auth imports `next/server`, which doesn't resolve outside Next; only the matcher is under test.
vi.mock('next-auth', () => ({ default: () => ({ auth: vi.fn() }) }))

describe('proxy matcher', () => {
  it.each([
    ['/dashboard', true],
    ['/dashboard/peers', true],
    ['/dashboard/invites', true],
    ['/login', false],
    ['/api/live', false],
  ])('runs on %s: %s', (url, matches) => {
    expect(unstable_doesMiddlewareMatch({ config, nextConfig, url })).toBe(matches)
  })
})
