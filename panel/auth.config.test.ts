import { describe, expect, it } from 'vitest'
import { authConfig } from './auth.config'

const { authorized, jwt, session } = authConfig.callbacks

type JwtParams = Parameters<typeof jwt>[0]
type SessionParams = Parameters<typeof session>[0]
type AuthorizedParams = Parameters<typeof authorized>[0]

const emptySession = () => ({ user: {}, expires: '2026-09-13T00:00:00Z' })

describe('jwt', () => {
  it('writes uid, role and name at sign-in', () => {
    const token = jwt({
      token: { name: 'Admin', sub: 'admin' },
      user: { id: 'admin', role: 'admin', name: 'Admin' },
    } as JwtParams)

    expect(token).toEqual({ sub: 'admin', uid: 'admin', role: 'admin', name: 'Admin' })
  })

  it('keeps the token as it is after sign-in', () => {
    const token = { uid: 'admin', role: 'admin' }
    expect(jwt({ token } as unknown as JwtParams)).toBe(token)
  })
})

describe('session', () => {
  it('reads the user from the claims', () => {
    const token = { uid: 'admin', role: 'admin', name: 'Admin' }
    const result = session({ session: emptySession(), token } as unknown as SessionParams)

    expect(result).toMatchObject({ user: { id: 'admin', role: 'admin', name: 'Admin' } })
  })
})

describe('authorized', () => {
  const request = (path: string) => ({ nextUrl: new URL(`https://panel.example.com${path}`) })

  it.each([
    ['an admin', { user: { role: 'admin' } }, true],
    ['a session without the admin role', { user: { role: 'user' } }, false],
    ['no session', null, false],
  ])('routes %s into /dashboard: %s', (_, auth, allowed) => {
    for (const path of ['/dashboard', '/dashboard/invites']) {
      expect(authorized({ auth, request: request(path) } as unknown as AuthorizedParams)).toBe(
        allowed,
      )
    }
  })

  it('lets a request without a session through outside /dashboard', () => {
    expect(
      authorized({ auth: null, request: request('/login') } as unknown as AuthorizedParams),
    ).toBe(true)
  })
})
