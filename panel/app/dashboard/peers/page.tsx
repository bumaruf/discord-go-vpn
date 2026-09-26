import { Suspense } from 'react'
import { listDevices, type Device } from '@/lib/devices'
import { inviteLabelsById } from '@/lib/invites'
import { DeviceList } from './device-list'
import type { InviteLabels } from './invite-groups'

export const dynamic = 'force-dynamic'

export default async function PeersPage() {
  const [devices, inviteLabels] = await Promise.all([loadDevices(), loadInviteLabels()])

  // Without a Suspense boundary, useSearchParams makes the whole page render only in the browser.
  return (
    <Suspense fallback={null}>
      <DeviceList devices={devices} inviteLabels={inviteLabels} />
    </Suspense>
  )
}

/** `null` says the database didn't answer, so the list shows only who is on the stream. */
async function loadDevices(): Promise<Device[] | null> {
  try {
    return await listDevices()
  } catch (error) {
    console.error('failed to read devices', error)
    return null
  }
}

async function loadInviteLabels(): Promise<InviteLabels> {
  try {
    return await inviteLabelsById()
  } catch (error) {
    // The list still works without labels, showing each group as a removed invite.
    console.error('failed to read invite labels', error)
    return {}
  }
}
