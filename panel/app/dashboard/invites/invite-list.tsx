'use client'

import { Ticket } from 'lucide-react'
import { useTransition } from 'react'
import { useLiveState } from '@/components/live-provider'
import { Card } from '@/components/ui/card'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { PageHeader } from '@/components/ui/page-header'
import { Table, TableBody, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { ActionResult } from '@/lib/action-result'
import { toastResult } from '@/lib/toast-result'
import { ROUTES } from '../routes'
import { deleteInvite, reactivateInvite, revokeInvite, setInviteMaxLeases } from './actions'
import { InviteTableRow, type InviteRow } from './invite-table-row'
import { NewInviteDialog } from './new-invite-dialog'
import { countOnlineByInvite } from './online-by-invite'

export function InviteList({ invites }: { invites: InviteRow[] }) {
  const { snapshot } = useLiveState()
  const [pending, startTransition] = useTransition()

  const onlineByInvite = countOnlineByInvite(snapshot.peers)
  const active = invites.filter((invite) => invite.status === 'active')
  const revoked = invites.filter((invite) => invite.status === 'revoked')

  function run(action: () => Promise<ActionResult>) {
    startTransition(async () => toastResult(await action()))
  }

  function table(rows: InviteRow[]) {
    return (
      <Card className="py-0 [&_td]:px-4 [&_td]:py-3 [&_th]:px-4">
        {/* Fixed widths keep the active and revoked tables aligned with each other. */}
        <Table className="table-fixed">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="h-10 text-muted-foreground">Convite</TableHead>
              <TableHead className="h-10 w-32 text-muted-foreground sm:w-48">Máquinas</TableHead>
              <TableHead className="hidden h-10 w-36 text-muted-foreground sm:table-cell">
                Criado em
              </TableHead>
              <TableHead className="h-10 w-14">
                <span className="sr-only">Ações</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((invite) => (
              <InviteTableRow
                key={invite.id}
                invite={invite}
                online={onlineByInvite.get(invite.id) ?? 0}
                pending={pending}
                onRevoke={() => run(() => revokeInvite(invite.id))}
                onReactivate={() => run(() => reactivateInvite(invite.id))}
                onDelete={() => run(() => deleteInvite(invite.id))}
                onSetMaxLeases={(raw) => run(() => setInviteMaxLeases(invite.id, raw))}
              />
            ))}
          </TableBody>
        </Table>
      </Card>
    )
  }

  return (
    <>
      <PageHeader
        title={ROUTES.invites.title}
        description="Quem tem o token de um convite ativo entra no túnel."
        actions={invites.length > 0 ? <NewInviteDialog /> : null}
      />

      {invites.length === 0 ? (
        <Card>
          <Empty className="py-16">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Ticket />
              </EmptyMedia>
              <EmptyTitle>Nenhum convite ainda</EmptyTitle>
              <EmptyDescription>
                Crie um convite e envie o token de conexão para quem vai usar a VPN.
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <NewInviteDialog />
            </EmptyContent>
          </Empty>
        </Card>
      ) : (
        <div className="flex flex-col gap-8">
          {active.length > 0 ? (
            table(active)
          ) : (
            <p className="text-sm text-muted-foreground">
              Nenhum convite ativo: ninguém consegue entrar até você criar ou reativar um.
            </p>
          )}

          {revoked.length > 0 ? (
            <section className="flex flex-col gap-3" aria-labelledby="revoked-invites">
              <div className="flex flex-col gap-0.5">
                <h2 id="revoked-invites" className="text-base font-medium">
                  Revogados
                </h2>
                <p className="text-sm text-muted-foreground">
                  Sem acesso até serem reativados. Exclua os que não vão voltar.
                </p>
              </div>
              {table(revoked)}
            </section>
          ) : null}
        </div>
      )}
    </>
  )
}
