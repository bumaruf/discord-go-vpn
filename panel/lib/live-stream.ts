import type { LiveSnapshot } from './live-snapshot'

export interface LiveStreamRead {
  snapshots: LiveSnapshot[]
  /** An event still incomplete; prepend it to the next chunk. */
  rest: string
}

/** Reads the complete events in `buffer`: a network chunk can split an SSE event. */
export function readLiveStream(buffer: string): LiveStreamRead {
  const snapshots: LiveSnapshot[] = []
  let rest = buffer
  let boundary = rest.indexOf('\n\n')

  while (boundary !== -1) {
    const event = rest.slice(0, boundary)
    rest = rest.slice(boundary + 2)
    boundary = rest.indexOf('\n\n')

    const lines = event.split('\n')
    // Snapshots are unnamed events; a named one, like the end event, carries no snapshot.
    if (lines.some((l) => l.startsWith('event:'))) continue

    const data = lines
      .filter((l) => l.startsWith('data: '))
      .map((l) => l.slice(6))
      .join('')

    if (!data) continue
    try {
      snapshots.push(JSON.parse(data) as LiveSnapshot)
    } catch {
      // Skip a malformed event instead of dropping the stream.
    }
  }

  return { snapshots, rest }
}
