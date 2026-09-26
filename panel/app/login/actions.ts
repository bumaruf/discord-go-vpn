'use server'

import { AuthError } from 'next-auth'
import { signIn, TooManyLoginAttempts } from '@/auth'
import { LOCKOUT_MINUTES } from '@/lib/login-attempts'

export type LoginState = { error?: string }

export async function authenticate(_previous: LoginState, formData: FormData): Promise<LoginState> {
  const username = String(formData.get('username') ?? '').trim()
  const password = String(formData.get('senha') ?? '')

  if (!username || !password) {
    return { error: 'Preencha usuário e senha.' }
  }

  try {
    await signIn('credentials', { username, senha: password, redirectTo: '/dashboard' })
  } catch (error) {
    if (error instanceof TooManyLoginAttempts) {
      return {
        error: `Tentativas erradas demais. Tente de novo em ${LOCKOUT_MINUTES} minutos.`,
      }
    }
    if (error instanceof AuthError) {
      return { error: 'Usuário ou senha incorretos.' }
    }
    // A successful sign-in redirects by throwing, so it must propagate.
    throw error
  }

  return {}
}
