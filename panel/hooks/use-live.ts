'use client'

import { useEffect, useRef, useState } from 'react'
import { EMPTY_SNAPSHOT, type LiveSource, type LiveSnapshot } from '@/lib/live-snapshot'
import { readLiveStream } from '@/lib/live-stream'

/** One minute of one-second samples. */
export const WINDOW_SIZE = 60

/** Failures before the header says the live view is down instead of reconnecting. */
const FAILURES_BEFORE_OFFLINE = 3

export interface ThroughputPoint {
  down: number
  up: number
}

export interface LiveState {
  snapshot: LiveSnapshot
  history: ThroughputPoint[]
  source: LiveSource
}

/**
 * Streams snapshots straight from provisiond, without going through the panel's server or the
 * database. While the stream is down it keeps retrying with backoff, showing the last snapshot.
 */
export function useLive(initial?: LiveSnapshot): LiveState {
  const [snapshot, setSnapshot] = useState<LiveSnapshot>(initial ?? EMPTY_SNAPSHOT)
  const [source, setSource] = useState<LiveSource>('connecting')
  const history = useRef<ThroughputPoint[]>([])

  useEffect(() => {
    // One controller ends the stream and the backoff wait on unmount.
    const controller = new AbortController()
    let failures = 0

    function record(s: LiveSnapshot) {
      history.current = [...history.current, { down: s.down_bps, up: s.up_bps }].slice(-WINDOW_SIZE)
      setSnapshot(s)
    }

    async function openStream(): Promise<void> {
      const ticketResponse = await fetch('/api/ticket', { signal: controller.signal })
      if (!ticketResponse.ok) throw new Error('no ticket')

      const { ticket, url } = (await ticketResponse.json()) as { ticket: string; url: string }

      const response = await fetch(url, {
        headers: { Authorization: `Bearer ${ticket}` },
        signal: controller.signal,
      })
      if (!response.ok || !response.body) throw new Error(`stream refused: ${response.status}`)

      failures = 0
      setSource('live')

      const reader = response.body.pipeThrough(new TextDecoderStream()).getReader()
      let buffer = ''

      for (;;) {
        const { done, value } = await reader.read()
        if (done) break

        const read = readLiveStream(buffer + value)
        buffer = read.rest
        read.snapshots.forEach(record)
      }

      // provisiond ends the stream periodically to force a ticket renewal; getting here is normal.
    }

    async function loop() {
      while (!controller.signal.aborted) {
        try {
          await openStream()
        } catch (error) {
          if (controller.signal.aborted) return
          failures++
          setSource(failures >= FAILURES_BEFORE_OFFLINE ? 'offline' : 'connecting')
          console.debug('live stream unavailable', error)
        }

        if (controller.signal.aborted) return

        const delay = Math.min(1000 * 2 ** Math.min(failures, 4), 10_000)
        await new Promise((r) => setTimeout(r, failures === 0 ? 250 : delay))
      }
    }

    void loop()
    return () => controller.abort()
  }, [])

  return { snapshot, history: history.current, source }
}
