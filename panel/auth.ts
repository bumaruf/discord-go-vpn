import NextAuth, { CredentialsSignin } from 'next-auth'
import Credentials from 'next-auth/providers/credentials'
import { z } from 'zod'
import { authConfig } from './auth.config'
import { verifyAdmin } from './lib/admin'
import {
  clearFailedLogins,
  clientAddress,
  loginBlocked,
  recordFailedLogin,
} from './lib/login-attempts'

export class TooManyLoginAttempts extends CredentialsSignin {
  code = 'too_many_attempts'
}

const credentialsSchema = z.object({
  username: z.string().min(1).max(64),
  senha: z.string().min(1).max(256),
})

export const { handlers, auth, signIn, signOut } = NextAuth({
  ...authConfig,
  providers: [
    Credentials({
      credentials: { username: {}, senha: {} },

      // Checked here rather than in the login form's action, since the credentials callback
      // route can be posted to directly.
      async authorize(raw, request) {
        const address = clientAddress(request)
        if (await loginBlocked(address)) throw new TooManyLoginAttempts()

        const parsed = credentialsSchema.safeParse(raw)
        const admin = parsed.success
          ? await verifyAdmin(parsed.data.username, parsed.data.senha)
          : null
        if (!admin) {
          await recordFailedLogin(address)
          return null
        }

        await clearFailedLogins(address)
        return {
          id: admin.id,
          role: 'admin' as const,
          name: admin.name,
        }
      },
    }),
  ],
})
