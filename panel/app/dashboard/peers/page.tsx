import { Suspense } from 'react'
import { listDevices, type DeviceDirectory } from '@/lib/devices'
import { DeviceList } from './device-list'

export const dynamic = 'force-dynamic'

export default async function PeersPage() {
  const directory = await loadDevices()

  // Without a Suspense boundary, useSearchParams makes the whole page render only in the browser.
  return (
    <Suspense fallback={null}>
      <DeviceList
        devices={directory?.devices ?? null}
        inviteLabels={directory?.inviteLabels ?? {}}
      />
    </Suspense>
  )
}

/**
 * `null` says the database didn't answer, so the list shows only who is on the stream, each group
 * as a removed invite.
 */
async function loadDevices(): Promise<DeviceDirectory | null> {
  try {
    return await listDevices()
  } catch (error) {
    console.error('failed to read devices', error)
    return null
  }
}
