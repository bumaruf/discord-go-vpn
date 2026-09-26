import { CircleAlert } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { PageHeader } from '@/components/ui/page-header'
import { countDevicesByInvite } from '@/lib/devices'
import { listRecentInvites } from '@/lib/invites'
import { ROUTES } from '../routes'
import { InviteList } from './invite-list'
import type { InviteRow } from './invite-table-row'

export const dynamic = 'force-dynamic'

const shortDate = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short' })

export default async function InvitesPage() {
  const rows = await loadInviteRows()

  // The try/catch covers only the fetch: around the render it would swallow errors meant for the
  // error boundary.
  if (!rows) {
    return (
      <>
        <PageHeader title={ROUTES.invites.title} />
        <Alert variant="destructive">
          <CircleAlert />
          <AlertTitle>O banco não respondeu</AlertTitle>
          <AlertDescription>
            Os convites existentes continuam valendo: o servidor não depende desta tela para
            autorizar ninguém. Recarregue a página para tentar de novo.
          </AlertDescription>
        </Alert>
      </>
    )
  }

  return <InviteList invites={rows} />
}

async function loadInviteRows(): Promise<InviteRow[] | null> {
  try {
    const invites = await listRecentInvites()
    // One aggregation for every invite instead of one count per row.
    const deviceCounts = await countDevicesByInvite(invites.map((invite) => invite.id))

    return invites.map((invite): InviteRow => ({
      id: invite.id,
      label: invite.label,
      kind: invite.kind,
      status: invite.active ? 'active' : 'revoked',
      createdAt: shortDate.format(invite.createdAt),
      deviceCount: deviceCounts.get(invite.id) ?? 0,
      maxLeases: invite.maxLeases,
    }))
  } catch (error) {
    console.error('failed to list invites', error)
    return null
  }
}
