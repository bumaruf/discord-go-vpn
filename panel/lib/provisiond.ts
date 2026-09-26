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

async function callProvisiond(path: string): Promise<ProvisiondResult> {
  const url = provisiondUrl(path)
  const secret = panelSecret()

  if (!url || !secret) {
    return { ok: false, error: 'As ações não estão configuradas neste painel.' }
  }

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: { Authorization: `Bearer ${secret}` },
      cache: 'no-store',
      signal: AbortSignal.timeout(TIMEOUT_MS),
    })

    if (!response.ok) {
      return { ok: false, error: await describeFailure(response) }
    }

    const body = (await response.json()) as ActionResponse
    return { ok: true, message: body.message, affected: body.affected ?? 0 }
  } catch (error) {
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
