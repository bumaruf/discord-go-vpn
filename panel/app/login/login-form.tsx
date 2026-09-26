'use client'

import { CircleAlert } from 'lucide-react'
import { useActionState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardFooter } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SESSION_MAX_AGE_HOURS } from '@/lib/session-max-age'
import { authenticate, type LoginState } from './actions'

const INITIAL_STATE: LoginState = {}

export function LoginForm() {
  const [state, formAction, pending] = useActionState(authenticate, INITIAL_STATE)

  return (
    <Card>
      <CardContent>
        <form className="flex flex-col gap-4" action={formAction}>
          <div className="grid gap-2">
            <Label htmlFor="username">Usuário</Label>
            <Input
              id="username"
              name="username"
              type="text"
              autoComplete="username"
              autoFocus
              disabled={pending}
              className="h-9"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="senha">Senha</Label>
            <Input
              id="senha"
              name="senha"
              type="password"
              autoComplete="current-password"
              disabled={pending}
              className="h-9"
            />
          </div>

          {state.error ? (
            <Alert variant="destructive">
              <CircleAlert />
              <AlertDescription>{state.error}</AlertDescription>
            </Alert>
          ) : null}

          <Button type="submit" size="lg" disabled={pending}>
            {pending ? 'Verificando…' : 'Entrar'}
          </Button>
        </form>
      </CardContent>
      <CardFooter className="justify-center text-xs text-muted-foreground">
        A sessão expira em {SESSION_MAX_AGE_HOURS}h
      </CardFooter>
    </Card>
  )
}
