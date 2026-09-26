import { ShieldCheck } from 'lucide-react'
import { cn } from '@/lib/utils'

export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'flex size-7 shrink-0 items-center justify-center rounded-md bg-foreground text-background [&_svg]:size-4',
        className,
      )}
      aria-hidden
    >
      <ShieldCheck />
    </span>
  )
}
