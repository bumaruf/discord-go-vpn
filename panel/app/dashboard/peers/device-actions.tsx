'use client'

import Link from 'next/link'
import { useTransition } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toastResult } from '@/lib/toast-result'
import { ROUTES } from '../routes'
import { disconnectPeer } from './actions'
import type { ListedDevice } from './listed-devices'

export function DeviceActions({ device }: { device: ListedDevice }) {
  const [pending, startTransition] = useTransition()
  const leaseId = device.peer?.id

  // A machine that is neither on the tunnel nor on an invite has nothing to act on.
  if (!leaseId && !device.inviteId) return null

  function disconnect() {
    startTransition(async () => toastResult(await disconnectPeer(leaseId!)))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Ações</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col divide-y">
        {leaseId ? (
          <ActionRow
            title="Desconectar"
            description="Derruba o túnel agora. É temporário: o convite continua valendo e a máquina reconecta em segundos."
          >
            <Button variant="outline" disabled={pending} onClick={disconnect}>
              {pending ? 'Desconectando…' : 'Desconectar'}
            </Button>
          </ActionRow>
        ) : null}

        {device.inviteId ? (
          <ActionRow
            title="Revogar o convite"
            description="Tira o acesso de vez, de todas as máquinas que entraram pelo mesmo convite. A revogação é feita na página de convites."
          >
            <Button variant="destructive" asChild>
              <Link href={ROUTES.invites.href}>Abrir convites</Link>
            </Button>
          </ActionRow>
        ) : null}
      </CardContent>
    </Card>
  )
}

function ActionRow({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between sm:gap-8">
      <div className="flex max-w-prose flex-col gap-1">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}
