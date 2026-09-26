'use client'

import { Check, Copy, Plus, User, Users } from 'lucide-react'
import { useRouter } from 'next/navigation'
import { useState, useTransition } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import type { InviteKind } from '@/lib/invites'
import { MAX_LEASES_LIMIT } from '@/lib/max-leases'
import { INVITE_KIND } from '@/lib/mongo-names'
import { toastResult } from '@/lib/toast-result'
import { cn } from '@/lib/utils'
import { createInvite } from './actions'

const KIND_OPTIONS: { kind: InviteKind; title: string; description: string; icon: typeof User }[] =
  [
    {
      kind: INVITE_KIND.individual,
      title: 'Individual',
      description: 'Para uma pessoa. Revogar tira o acesso só dela.',
      icon: User,
    },
    {
      kind: INVITE_KIND.global,
      title: 'Global',
      description: 'Um token para um grupo. Revogar derruba todo mundo que usa.',
      icon: Users,
    },
  ]

export function NewInviteDialog() {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<{ token: string; label: string } | null>(null)

  // Closing forgets the token: it exists only in this dialog, never again in the panel. The new
  // invite reaches the list here, and not when it is created, so the list re-rendering can't
  // unmount this dialog while the token is on screen.
  function onOpenChange(next: boolean) {
    setOpen(next)
    if (next) return

    setCreated(null)
    if (created) router.refresh()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        <Button size="lg">
          <Plus aria-hidden />
          Novo convite
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        {created ? (
          <TokenStep
            token={created.token}
            label={created.label}
            onDone={() => onOpenChange(false)}
          />
        ) : (
          <CreateStep onCreated={setCreated} />
        )}
      </DialogContent>
    </Dialog>
  )
}

function CreateStep({
  onCreated,
}: {
  onCreated: (created: { token: string; label: string }) => void
}) {
  const [pending, startTransition] = useTransition()
  const [kind, setKind] = useState<InviteKind>(INVITE_KIND.individual)

  return (
    <form
      className="flex flex-col gap-5"
      action={(formData) =>
        startTransition(async () => {
          const result = await createInvite(formData)
          if (result.ok) {
            onCreated({ token: result.token, label: String(formData.get('rotulo') ?? '') })
          } else {
            toastResult(result)
          }
        })
      }
    >
      <DialogHeader>
        <DialogTitle>Novo convite</DialogTitle>
        <DialogDescription>
          O convite gera um token de conexão. Quem tiver o token entra no túnel enquanto o convite
          estiver ativo.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2">
        <Label htmlFor="invite-label">Para quem é</Label>
        <Input
          id="invite-label"
          name="rotulo"
          placeholder="Ex.: Ana, turma A"
          maxLength={60}
          autoComplete="off"
          disabled={pending}
        />
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="invite-max-leases">Conexões simultâneas</Label>
        <Input
          id="invite-max-leases"
          name="maxLeases"
          type="number"
          inputMode="numeric"
          min={1}
          max={MAX_LEASES_LIMIT}
          placeholder="Sem limite"
          disabled={pending}
          className="sm:w-40"
        />
        <p className="text-sm text-muted-foreground">
          Quantas máquinas podem usar o token ao mesmo tempo. Vazio: sem limite.
        </p>
      </div>

      <div className="flex flex-col gap-2">
        <Label id="invite-kind-label">Tipo</Label>
        <RadioGroup
          name="kind"
          value={kind}
          onValueChange={(value) => setKind(value as InviteKind)}
          aria-labelledby="invite-kind-label"
          className="grid-cols-1 sm:grid-cols-2"
          disabled={pending}
        >
          {KIND_OPTIONS.map((option) => (
            <Label
              key={option.kind}
              htmlFor={`invite-kind-${option.kind}`}
              className={cn(
                'flex cursor-pointer flex-col items-start gap-2 rounded-lg border p-3 font-normal transition-colors hover:bg-muted/50',
                kind === option.kind && 'border-primary bg-muted/50',
              )}
            >
              <span className="flex w-full items-center justify-between">
                <span className="flex items-center gap-2 font-medium">
                  <option.icon className="size-4 text-muted-foreground" aria-hidden />
                  {option.title}
                </span>
                <RadioGroupItem id={`invite-kind-${option.kind}`} value={option.kind} />
              </span>
              <span className="text-sm leading-snug text-muted-foreground">
                {option.description}
              </span>
            </Label>
          ))}
        </RadioGroup>
      </div>

      <DialogFooter>
        <Button type="submit" disabled={pending}>
          {pending ? 'Gerando…' : 'Gerar token'}
        </Button>
      </DialogFooter>
    </form>
  )
}

function TokenStep({ token, label, onDone }: { token: string; label: string; onDone: () => void }) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(token)
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch {
      toast.error('O token não foi copiado. Selecione o texto e copie à mão.')
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>Token de {label || 'sem nome'}</DialogTitle>
        <DialogDescription>
          Copie e envie agora: este token não aparece de novo. Se ele se perder, exclua o convite e
          gere outro.
        </DialogDescription>
      </DialogHeader>

      <code className="block max-h-40 overflow-y-auto rounded-md border bg-muted/40 px-3 py-2.5 font-mono text-xs break-all select-all">
        {token}
      </code>

      <p className="text-sm text-muted-foreground">
        Quem receber abre o programa e cola o token quando ele pedir.
      </p>

      <DialogFooter>
        <Button variant="outline" onClick={onDone}>
          Concluir
        </Button>
        <Button onClick={copy}>
          {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
          {copied ? 'Copiado' : 'Copiar token'}
        </Button>
      </DialogFooter>
    </div>
  )
}
