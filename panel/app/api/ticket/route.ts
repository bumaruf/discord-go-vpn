import { errorResponse, ticketResponse } from '@/lib/api-response'
import { provisiondUrl } from '@/lib/provisiond-url'
import { deniedResponse, requireAdmin } from '@/lib/require-admin'
import { issueTicket } from '@/lib/ticket'

export const runtime = 'nodejs'
export const dynamic = 'force-dynamic'

/** Also returns the stream URL, so the provisiond address needn't be a NEXT_PUBLIC_ variable. */
export async function GET() {
  const admin = await requireAdmin()
  if (!admin.ok) return deniedResponse(admin.status)

  const url = provisiondUrl('/admin/live')
  if (!url) return errorResponse('PROVISIOND_URL não configurada', 503)

  try {
    return ticketResponse(issueTicket(admin.user.id, admin.user.role), url)
  } catch {
    return errorResponse('stream não configurado', 503)
  }
}
