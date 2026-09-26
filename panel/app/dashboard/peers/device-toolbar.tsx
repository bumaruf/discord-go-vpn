'use client'

import { ListFilter, Search, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { hiddenFilterCount, type DeviceFilters, type StatusFilter } from './device-filters'

const STATUS_LABELS: Record<StatusFilter, string> = {
  all: 'Todas',
  online: 'No ar',
  offline: 'Fora do ar',
}

export interface InviteOption {
  id: string
  label: string
  count: number
}

export function DeviceToolbar({
  filters,
  invites,
  shown,
  total,
  onChange,
}: {
  filters: DeviceFilters
  invites: InviteOption[]
  shown: number
  total: number
  onChange: (filters: DeviceFilters) => void
}) {
  const update = (change: Partial<DeviceFilters>) => onChange({ ...filters, ...change })
  const toggleInvite = (id: string, checked: boolean) =>
    update({
      invites: checked ? [...filters.invites, id] : filters.invites.filter((other) => other !== id),
    })
  const hidden = hiddenFilterCount(filters)
  const selectedInvites = invites.filter((invite) => filters.invites.includes(invite.id))

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            value={filters.query}
            onChange={(event) => update({ query: event.target.value })}
            placeholder="Buscar máquina, usuário, IP ou convite…"
            aria-label="Buscar máquinas"
            className="pl-8"
          />
        </div>

        <Tabs
          value={filters.status}
          onValueChange={(status) => update({ status: status as StatusFilter })}
        >
          <TabsList>
            {Object.entries(STATUS_LABELS).map(([status, label]) => (
              <TabsTrigger key={status} value={status}>
                {label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline">
              <ListFilter />
              Filtros
              {hidden > 0 ? (
                <Badge variant="secondary" className="tabular-nums">
                  {hidden}
                </Badge>
              ) : null}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-60">
            <DropdownMenuLabel>Convite</DropdownMenuLabel>
            {invites.map((invite) => (
              <DropdownMenuCheckboxItem
                key={invite.id}
                checked={filters.invites.includes(invite.id)}
                onCheckedChange={(checked) => toggleInvite(invite.id, checked)}
                // Keeps the menu open, so several invites can be ticked in one go.
                onSelect={(event) => event.preventDefault()}
              >
                <span className="truncate">{invite.label}</span>
                <span className="ml-auto text-muted-foreground tabular-nums">{invite.count}</span>
              </DropdownMenuCheckboxItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuCheckboxItem
              checked={filters.group}
              onCheckedChange={(group) => update({ group })}
            >
              Agrupar por convite
            </DropdownMenuCheckboxItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {shown < total ? (
          <span className="ml-auto text-sm text-muted-foreground tabular-nums">
            {shown} de {total}
          </span>
        ) : null}
      </div>

      {hidden > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          {selectedInvites.map((invite) => (
            <FilterChip
              key={invite.id}
              label={`Convite: ${invite.label}`}
              onRemove={() => toggleInvite(invite.id, false)}
            />
          ))}
          {filters.group ? (
            <FilterChip label="Agrupado por convite" onRemove={() => update({ group: false })} />
          ) : null}
          <Button
            variant="link"
            size="sm"
            className="h-auto px-1 text-muted-foreground"
            onClick={() => update({ invites: [], group: false })}
          >
            Limpar filtros
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <Badge variant="outline" className="h-6 gap-1 pr-1">
      {label}
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Remover ${label}`}
        className="rounded-full p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        <X className="size-3" />
      </button>
    </Badge>
  )
}
