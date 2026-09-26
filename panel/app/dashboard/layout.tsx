import { redirect } from 'next/navigation'
import { LiveProvider } from '@/components/live-provider'
import { Toaster } from '@/components/ui/sonner'
import { serverName } from '@/lib/env'
import { requireAdmin } from '@/lib/require-admin'
import { Header } from './header'

/** Checked here too: the proxy only routes, and its matcher can change. */
export default async function PanelLayout({ children }: { children: React.ReactNode }) {
  const admin = await requireAdmin()
  if (!admin.ok) redirect('/login')

  return (
    <LiveProvider>
      <div className="flex min-h-screen flex-col">
        <Header serverName={serverName()} />
        <main className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-8 px-4 py-8 sm:px-6 sm:py-10">
          {children}
        </main>
      </div>
      <Toaster position="bottom-right" />
    </LiveProvider>
  )
}
