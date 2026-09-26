'use client'

import { ArrowDown, ArrowUp, ChevronsUpDown, CircleAlert, MonitorOff, SearchX } from 'lucide-react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Fragment } from 'react'
import { useLiveState } from '@/components/live-provider'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { PageHeader } from '@/components/ui/page-header'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { Device } from '@/lib/devices'
import { cn } from '@/lib/utils'
import { ROUTES } from '../routes'
import { DeviceDetail } from './device-detail'
import {
  filterDevices,
  filtersToParams,
  parseFilters,
  sortDevices,
  type DeviceFilters,
  type SortKey,
} from './device-filters'
import { DeviceRow } from './device-row'
import { DeviceToolbar } from './device-toolbar'
import { groupByInvite, inviteLabel, type InviteLabels } from './invite-groups'
import { listedDevices, type ListedDevice } from './listed-devices'

const DEVICE_TABLE_HEADERS: { label: string; sort?: SortKey }[] = [
  { label: 'Máquina', sort: 'name' },
  { label: 'Origem' },
  { label: 'IP interno' },
  { label: 'Tráfego', sort: 'traffic' },
  { label: 'Sessão', sort: 'session' },
]

const machines = (count: number) => `${count} ${count === 1 ? 'máquina' : 'máquinas'}`

/** `devices` is `null` when the database didn't answer: only who is on the stream is left. */
export function DeviceList({
  devices,
  inviteLabels,
}: {
  devices: Device[] | null
  inviteLabels: InviteLabels
}) {
  const { snapshot } = useLiveState()
  const params = useSearchParams()
  const router = useRouter()

  const listed = listedDevices(devices ?? [], snapshot.peers)
  const selectedId = params.get('device')
  const selected = selectedId ? listed.find((device) => device.id === selectedId) : undefined

  const listParams = new URLSearchParams(params)
  listParams.delete('device')
  listParams.delete('peer')
  const withQuery = (query: URLSearchParams) =>
    query.size > 0 ? `${ROUTES.peers.href}?${query}` : ROUTES.peers.href

  // The detail replaces the list, and the id lives in the query string so it can be linked.
  if (selectedId && selected) {
    return (
      <DeviceDetail
        device={selected}
        inviteLabel={inviteLabel(selected.inviteId, inviteLabels)}
        backHref={withQuery(listParams)}
      />
    )
  }

  const filters = parseFilters(params)
  // Only the query string changes, so the server isn't asked for the page again on each keystroke.
  const setFilters = (next: DeviceFilters) =>
    window.history.replaceState(null, '', withQuery(filtersToParams(next, listParams)))
  const clearFilters = () =>
    setFilters({ ...filters, query: '', status: 'all', invites: [], group: false })

  const shown = sortDevices(filterDevices(listed, filters, inviteLabels), filters)
  const groups = filters.group
    ? groupByInvite(shown, inviteLabels)
    : [{ id: 'all', label: '', devices: shown }]

  const toggleSort = (sort: SortKey) =>
    setFilters({
      ...filters,
      sort,
      // Names read best from A, numbers from the largest.
      dir:
        filters.sort === sort
          ? filters.dir === 'asc'
            ? 'desc'
            : 'asc'
          : sort === 'name'
            ? 'asc'
            : 'desc',
    })

  const rows = (group: ListedDevice[]) =>
    group.map((device) => {
      const query = new URLSearchParams(listParams)
      query.set('device', device.id)
      const href = withQuery(query)
      return (
        <DeviceRow
          key={device.id}
          device={device}
          href={href}
          onOpen={() => router.push(href)}
          invite={filters.group ? undefined : inviteLabel(device.inviteId, inviteLabels)}
        />
      )
    })

  return (
    <>
      <PageHeader
        title={ROUTES.peers.title}
        description={`${machines(listed.length)} com acesso, ${snapshot.online} no ar agora.`}
      />

      {devices === null ? (
        <Alert variant="destructive">
          <CircleAlert />
          <AlertTitle>O banco não respondeu</AlertTitle>
          <AlertDescription>
            A lista abaixo tem só quem está conectado agora, que vem do servidor. As máquinas
            desconectadas e o histórico voltam quando o banco responder.
          </AlertDescription>
        </Alert>
      ) : null}

      {listed.length > 0 ? (
        <DeviceToolbar
          filters={filters}
          invites={groupByInvite(listed, inviteLabels).map((group) => ({
            id: group.id,
            label: group.label,
            count: group.devices.length,
          }))}
          shown={shown.length}
          total={listed.length}
          onChange={setFilters}
        />
      ) : null}

      <Card className="py-0 [&_td]:px-4 [&_th]:px-4">
        {listed.length === 0 ? (
          <Empty className="py-16">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <MonitorOff />
              </EmptyMedia>
              <EmptyTitle>Nenhuma máquina por aqui</EmptyTitle>
              <EmptyDescription>
                Cada máquina aparece nesta lista a partir da primeira vez que entra com um convite,
                e continua nela depois de desconectar.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : shown.length === 0 ? (
          <Empty className="py-16">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <SearchX />
              </EmptyMedia>
              <EmptyTitle>Nenhuma máquina encontrada</EmptyTitle>
              <EmptyDescription>Nenhuma máquina combina com a busca e os filtros.</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button variant="outline" size="sm" onClick={clearFilters}>
                Limpar busca e filtros
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                {DEVICE_TABLE_HEADERS.map(({ label, sort }) => (
                  <TableHead
                    key={label}
                    className="h-10 text-muted-foreground"
                    aria-sort={
                      sort && filters.sort === sort
                        ? filters.dir === 'asc'
                          ? 'ascending'
                          : 'descending'
                        : undefined
                    }
                  >
                    {sort ? (
                      <button
                        type="button"
                        onClick={() => toggleSort(sort)}
                        className="-mx-1 inline-flex items-center gap-1 rounded px-1 hover:text-foreground"
                      >
                        {label}
                        <SortIcon active={filters.sort === sort} dir={filters.dir} />
                      </button>
                    ) : (
                      label
                    )}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((group) => (
                <Fragment key={group.id}>
                  {filters.group ? (
                    <TableRow className="bg-muted/40 hover:bg-muted/40">
                      <TableCell
                        colSpan={DEVICE_TABLE_HEADERS.length}
                        className={cn(
                          'py-2 text-xs font-medium',
                          group.id === '' && 'font-normal text-muted-foreground',
                        )}
                      >
                        {group.label}
                        <span className="ml-2 font-normal text-muted-foreground tabular-nums">
                          {machines(group.devices.length)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ) : null}
                  {rows(group.devices)}
                </Fragment>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </>
  )
}

function SortIcon({ active, dir }: { active: boolean; dir: DeviceFilters['dir'] }) {
  const Icon = !active ? ChevronsUpDown : dir === 'asc' ? ArrowUp : ArrowDown
  return <Icon className={cn('size-3.5', !active && 'opacity-50')} aria-hidden />
}
