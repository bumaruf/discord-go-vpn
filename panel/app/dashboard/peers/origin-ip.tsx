'use client'

import { Eye, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { maskIp } from '@/lib/format'
import { cn } from '@/lib/utils'

/** The machine's public address, masked until the admin asks to see it. */
export function OriginIp({
  ip,
  masked,
  className,
}: {
  ip?: string
  masked?: string
  className?: string
}) {
  const [revealed, setRevealed] = useState(false)

  if (!ip) return <span className={cn('font-mono text-xs', className)}>{masked || '—'}</span>

  return (
    <span className={cn('inline-flex items-center gap-1 font-mono text-xs', className)}>
      {/* Room for a full IPv4, so revealing it doesn't shift the table's columns. */}
      <span className="min-w-[15ch]">{revealed ? ip : maskIp(ip)}</span>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={revealed ? 'Ocultar IP de origem' : 'Mostrar IP de origem'}
        aria-pressed={revealed}
        // Inside a table row the click would also open the machine.
        onClick={(event) => {
          event.stopPropagation()
          setRevealed(!revealed)
        }}
      >
        {revealed ? <EyeOff /> : <Eye />}
      </Button>
    </span>
  )
}
