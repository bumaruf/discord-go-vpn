import 'server-only'

import type { ActionResult } from './action-result'
import { panelSecret } from './env'
import { provisiondUrl } from './provisiond-url'

type ProvisiondResult = ActionResult<{ affected: number }>

/** The admin is watching the click: failing beats a stuck screen. */
const TIMEOUT_MS = 5000

export interface ActionResponse {
  ok: boolean
  message: string
  affected: number
  executed_at: string
}

export class ProvisiondNotConfigured extends Error {
  constructor() {
    super('set PROVISIOND_URL and VPN_PANEL_SECRET')
  }
}

type Method = 'GET' | 'POST' | 'PUT' | 'DELETE'

async function send(method: Method, path: string, body?: unknown): Promise<Response> {
  const url = provisiondUrl(path)
  const secret = panelSecret()
  if (!url || !secret) throw new ProvisiondNotConfigured()

  return fetch(url, {
    method,
    headers: {
      Authorization: `Bearer ${secret}`,
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
    signal: AbortSignal.timeout(TIMEOUT_MS),
  })
}

/** Throws when provisiond or its database doesn't answer. */
export async function readProvisiond<T>(path: string): Promise<T> {
  const response = await send('GET', path)
  if (!response.ok) {
    throw new Error(`provisiond answered ${response.status} to GET ${path}`)
  }
  return (await response.json()) as T
}

/** False when the route answers 404, which the invite routes do for an invite that doesn't exist. */
export async function writeProvisiond(
  method: Method,
  path: string,
  body?: unknown,
): Promise<boolean> {
  const response = await send(method, path, body)
  if (response.status === 404) return false
  if (!response.ok) {
    throw new Error(`provisiond answered ${response.status} to ${method} ${path}`)
  }
  return true
}

async function callProvisiond(path: string): Promise<ProvisiondResult> {
  try {
    const response = await send('POST', path)
    if (!response.ok) {
      return { ok: false, error: await describeFailure(response) }
    }

    const body = (await response.json()) as ActionResponse
    return { ok: true, message: body.message, affected: body.affected ?? 0 }
  } catch (error) {
    if (error instanceof ProvisiondNotConfigured) {
      return { ok: false, error: 'As ações não estão configuradas neste painel.' }
    }
    if (error instanceof DOMException && error.name === 'TimeoutError') {
      return { ok: false, error: 'O servidor não respondeu a tempo.' }
    }
    return { ok: false, error: 'Não consegui falar com o servidor.' }
  }
}

async function describeFailure(response: Response): Promise<string> {
  switch (response.status) {
    case 401:
      return 'O servidor recusou a secret do painel.'
    case 404:
      return 'As ações não estão habilitadas no servidor.'
    case 503:
      return 'O servidor está sem banco e não consegue mexer em convites.'
    default: {
      const text = (await response.text()).trim()
      return text || `O servidor respondeu ${response.status}.`
    }
  }
}

export function disconnectLease(leaseId: string): Promise<ProvisiondResult> {
  return callProvisiond(`/admin/leases/${encodeURIComponent(leaseId)}/disconnect`)
}

/** Also drops every device of the invite that is online. */
export function revokeInviteOnServer(inviteId: string): Promise<ProvisiondResult> {
  return callProvisiond(`/admin/invites/${encodeURIComponent(inviteId)}/revoke`)
}

export function reactivateInviteOnServer(inviteId: string): Promise<ProvisiondResult> {
  return callProvisiond(`/admin/invites/${encodeURIComponent(inviteId)}/reactivate`)
}
