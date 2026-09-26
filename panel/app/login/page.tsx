import { redirect } from 'next/navigation'
import { auth } from '@/auth'
import { BrandMark } from '@/components/ui/brand-mark'
import { serverName } from '@/lib/env'
import { LoginForm } from './login-form'

export default async function LoginPage() {
  const session = await auth()
  if (session?.user) redirect('/dashboard')

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-12">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-4 text-center">
          <BrandMark className="size-10 rounded-lg [&_svg]:size-5" />
          <div className="flex flex-col gap-1">
            <h1 className="text-2xl font-semibold tracking-tight">Entrar no painel</h1>
            <p className="text-sm text-muted-foreground">
              Acesso de administrador do servidor{' '}
              <span className="font-mono text-foreground">{serverName()}</span>
            </p>
          </div>
        </div>

        <LoginForm />
      </div>
    </main>
  )
}
