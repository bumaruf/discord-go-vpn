import { NextResponse } from 'next/server'
import type { UsageSummary } from './usage'

const NO_STORE = { 'Cache-Control': 'no-store' }

export function errorResponse(error: string, status: number): NextResponse {
  return NextResponse.json({ error }, { status })
}

export function ticketResponse(ticket: string, url: string): NextResponse {
  return NextResponse.json({ ticket, url }, { headers: NO_STORE })
}

export function usageResponse(usage: UsageSummary): NextResponse {
  return NextResponse.json(usage, { headers: NO_STORE })
}
