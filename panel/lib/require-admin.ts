import type { NextResponse } from 'next/server'
import { errorResponse } from './api-response'
import type { Session } from 'next-auth'
import { auth } from '@/auth'

export type DeniedStatus = 401 | 403

export type AdminCheck = { ok: true; user: Session['user'] } | { ok: false; status: DeniedStatus }

export async function requireAdmin(): Promise<AdminCheck> {
  const session = await auth()
  if (!session?.user) return { ok: false, status: 401 }
  if (session.user.role !== 'admin') return { ok: false, status: 403 }
  return { ok: true, user: session.user }
}

const DENIED_ERROR: Record<DeniedStatus, string> = {
  401: 'não autenticado',
  403: 'sem permissão',
}

export function deniedResponse(status: DeniedStatus): NextResponse {
  return errorResponse(DENIED_ERROR[status], status)
}
