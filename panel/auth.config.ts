import type { NextAuthConfig } from 'next-auth'
import { SESSION_MAX_AGE_HOURS } from '@/lib/session-max-age'
import type { Role } from '@/lib/types'

/** Read from the JWT; Auth.js adds `name` itself, and the panel adds the rest. */
type TokenClaims = { uid?: string; role?: Role; name?: string | null }

/**
 * Shared by `proxy.ts` and `auth.ts`. Only `auth.ts` adds the credentials provider, so the proxy,
 * which just checks the signed JWT, never loads argon2 or the admin credentials.
 */
export const authConfig = {
  pages: {
    signIn: '/login',
  },
  session: {
    strategy: 'jwt',
    maxAge: SESSION_MAX_AGE_HOURS * 60 * 60,
  },
  callbacks: {
    /** Routing, not authorization: every Server Action and Route Handler checks the session. */
    authorized({ auth, request }) {
      const isAdmin = auth?.user?.role === 'admin'
      return request.nextUrl.pathname.startsWith('/dashboard') ? isAdmin : true
    },

    jwt({ token, user }) {
      if (!user) return token
      return { ...token, uid: user.id, role: user.role, name: user.name }
    },

    session({ session, token }) {
      // A cast, not module augmentation: the JWT type comes from `@auth/core/jwt`, which
      // augmentation doesn't reliably reach across next-auth betas.
      const t = token as TokenClaims
      if (t.uid) session.user.id = t.uid
      if (t.role) session.user.role = t.role
      if (t.name) session.user.name = t.name
      return session
    },
  },
  providers: [],
} satisfies NextAuthConfig
