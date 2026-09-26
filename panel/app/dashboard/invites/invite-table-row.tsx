'use client'

import { Ban, Ellipsis, Gauge, RotateCcw, Trash2, User, Users } from 'lucide-react'
import { useState } from 'react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { StatusDot } from '@/components/ui/status-dot'
import { TableCell, TableRow } from '@/components/ui/table'
import type { InviteKind } from '@/lib/invites'
import { MAX_LEASES_LIMIT } from '@/lib/max-leases'
import { cn } from '@/lib/utils'

const INVITE_KIND_LABELS: Record<InviteKind, string> = {
  individual: 'Individual',
  global: 'Global',
}

export interface InviteRow {
  id: string
  label: string
  kind: InviteKind
  status: 'active' | 'revoked'
  createdAt: string
  deviceCount: number
  maxLeases: number | null
}

type Confirmation = 'revoke' | 'delete'

export function InviteTableRow({
  invite,
  online,
  pending,
  onRevoke,
  onReactivate,
  onDelete,
  onSetMaxLeases,
}: {
  invite: InviteRow
  online: number
  pending: boolean
  onRevoke: () => void
  onReactivate: () => void
  onDelete: () => void
  onSetMaxLeases: (raw: string) => void
}) {
  const [confirming, setConfirming] = useState<Confirmation | null>(null)
  const [editingMaxLeases, setEditingMaxLeases] = useState(false)
  const active = invite.status === 'active'
  const KindIcon = invite.kind === 'global' ? Users : User

  return (
    <TableRow className={cn(!active && 'text-muted-foreground')}>
      <TableCell>
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className={cn('truncate font-medium', active && 'text-foreground')}>
            {invite.label}
          </span>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <KindIcon className="size-3.5" aria-hidden />
            {INVITE_KIND_LABELS[invite.kind]}
            <span aria-hidden>·</span>
            {invite.maxLeases === null ? 'sem limite' : `até ${invite.maxLeases} por vez`}
          </span>
        </div>
      </TableCell>

      <TableCell>
        <div className="flex items-center gap-3 tabular-nums">
          <span>{invite.deviceCount}</span>
          {online > 0 ? (
            <span className="flex items-center gap-1.5 text-xs text-success">
              <StatusDot tone="online" />
              {online} no ar
            </span>
          ) : null}
        </div>
      </TableCell>

      <TableCell className="hidden text-muted-foreground tabular-nums sm:table-cell">
        {invite.createdAt}
      </TableCell>

      <TableCell className="text-right">
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={pending}
              aria-label={`Ações do convite ${invite.label}`}
            >
              <Ellipsis aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {active ? (
              <DropdownMenuItem onSelect={() => setConfirming('revoke')}>
                <Ban aria-hidden />
                Revogar acesso
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onSelect={onReactivate}>
                <RotateCcw aria-hidden />
                Reativar
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onSelect={() => setEditingMaxLeases(true)}>
              <Gauge aria-hidden />
              Alterar limite
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={() => setConfirming('delete')}>
              <Trash2 aria-hidden />
              Excluir convite
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <AlertDialog
          open={confirming !== null}
          onOpenChange={(open) => {
            if (!open) setConfirming(null)
          }}
        >
          <AlertDialogContent>
            {confirming ? (
              <ConfirmationText kind={confirming} label={invite.label} online={online} />
            ) : null}
            <AlertDialogFooter>
              <AlertDialogCancel>Cancelar</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={confirming === 'delete' ? onDelete : onRevoke}
              >
                {confirming === 'delete' ? 'Excluir convite' : 'Revogar acesso'}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        <Dialog open={editingMaxLeases} onOpenChange={setEditingMaxLeases}>
          <DialogContent className="text-left sm:max-w-md">
            <form
              className="flex flex-col gap-5"
              action={(formData) => {
                setEditingMaxLeases(false)
                onSetMaxLeases(String(formData.get('maxLeases') ?? ''))
              }}
            >
              <DialogHeader>
                <DialogTitle>Limite de {invite.label}</DialogTitle>
                <DialogDescription>
                  Quantas máquinas podem usar o token ao mesmo tempo. Quem já está conectado não
                  cai; o limite vale para as próximas conexões.
                </DialogDescription>
              </DialogHeader>
              <div className="flex flex-col gap-2">
                <Label htmlFor={`max-leases-${invite.id}`}>Conexões simultâneas</Label>
                <Input
                  id={`max-leases-${invite.id}`}
                  name="maxLeases"
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={MAX_LEASES_LIMIT}
                  defaultValue={invite.maxLeases ?? ''}
                  placeholder="Sem limite"
                  autoFocus
                  className="sm:w-40"
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setEditingMaxLeases(false)}>
                  Cancelar
                </Button>
                <Button type="submit">Salvar</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </TableCell>
    </TableRow>
  )
}

function ConfirmationText({
  kind,
  label,
  online,
}: {
  kind: Confirmation
  label: string
  online: number
}) {
  const dropped =
    online === 0
      ? 'Nenhuma máquina está conectada por ele agora.'
      : `${online === 1 ? 'A máquina conectada' : `As ${online} máquinas conectadas`} por ele ${online === 1 ? 'cai' : 'caem'} na hora.`

  return (
    <AlertDialogHeader>
      <AlertDialogTitle>
        {kind === 'delete' ? `Excluir o convite ${label}?` : `Revogar o acesso de ${label}?`}
      </AlertDialogTitle>
      <AlertDialogDescription>
        {dropped}{' '}
        {kind === 'delete'
          ? 'O token deixa de funcionar e o convite some desta lista; o histórico de uso das máquinas continua no painel. Não dá para desfazer.'
          : 'O token para de funcionar até você reativar o convite.'}
      </AlertDialogDescription>
    </AlertDialogHeader>
  )
}
