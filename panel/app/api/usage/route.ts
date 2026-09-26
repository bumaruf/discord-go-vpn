import { errorResponse, usageResponse } from '@/lib/api-response'
import { deniedResponse, requireAdmin } from '@/lib/require-admin'
import { findDeviceUsage } from '@/lib/usage'

export const runtime = 'nodejs'
export const dynamic = 'force-dynamic'

/** A route because the device is picked in the browser, so histories aren't loaded upfront. */
export async function GET(request: Request) {
  const admin = await requireAdmin()
  if (!admin.ok) return deniedResponse(admin.status)

  const params = new URL(request.url).searchParams
  const deviceId = params.get('device')
  if (!deviceId) return errorResponse('informe o dispositivo', 400)

  const usage = await findDeviceUsage(deviceId)
  if (!usage) return errorResponse('banco indisponível', 503)

  return usageResponse(usage)
}
