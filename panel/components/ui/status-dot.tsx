import { cn } from '@/lib/utils'

export type StatusTone = 'online' | 'idle' | 'blocked' | 'offline'

const TONE_CLASS: Record<StatusTone, string> = {
  online: 'bg-success',
  idle: 'bg-warning',
  blocked: 'bg-destructive',
  offline: 'bg-muted-foreground/40',
}

export function StatusDot({
  tone = 'online',
  pulse = false,
  className,
}: {
  tone?: StatusTone
  pulse?: boolean
  className?: string
}) {
  return (
    <span className={cn('relative inline-flex size-2 shrink-0', className)} aria-hidden>
      {pulse ? (
        <span
          className={cn(
            'absolute inline-flex size-full animate-ping rounded-full opacity-60',
            TONE_CLASS[tone],
          )}
        />
      ) : null}
      <span className={cn('relative inline-flex size-2 rounded-full', TONE_CLASS[tone])} />
    </span>
  )
}
