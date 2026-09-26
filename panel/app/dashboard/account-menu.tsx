'use client'

import { LogOut, UserRound } from 'lucide-react'
import { useTransition } from 'react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { signOutAdmin } from './actions'

const ACCOUNT_LABEL = 'Administrador'

export function AccountMenu() {
  const [pending, startTransition] = useTransition()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="h-8 gap-2 rounded-full pr-2.5 pl-1">
          <span className="flex size-6 items-center justify-center rounded-full border bg-secondary">
            <UserRound className="size-3.5 text-muted-foreground" />
          </span>
          <span className="hidden sm:inline">{ACCOUNT_LABEL}</span>
          <span className="sr-only sm:hidden">Conta</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuLabel>{ACCOUNT_LABEL}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          disabled={pending}
          onSelect={(event) => {
            event.preventDefault()
            startTransition(() => signOutAdmin())
          }}
        >
          <LogOut />
          {pending ? 'Saindo…' : 'Sair'}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
