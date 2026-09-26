import { describe, expect, it } from 'vitest'
import { EMPTY_SNAPSHOT } from './live-snapshot'
import { readLiveStream } from './live-stream'

const snapshot = { ...EMPTY_SNAPSHOT, online: 1 }
const event = (data: unknown) => `data: ${JSON.stringify(data)}\n\n`

describe('readLiveStream', () => {
  it('reads every complete event in order', () => {
    const second = { ...snapshot, online: 2 }

    expect(readLiveStream(event(snapshot) + event(second))).toEqual({
      snapshots: [snapshot, second],
      rest: '',
    })
  })

  it('keeps an event split across chunks until it is complete', () => {
    const whole = event(snapshot)
    const first = readLiveStream(whole.slice(0, 10))

    expect(first).toEqual({ snapshots: [], rest: whole.slice(0, 10) })
    expect(readLiveStream(first.rest + whole.slice(10)).snapshots).toEqual([snapshot])
  })

  it('ignores the end event provisiond sends before closing the stream', () => {
    const end = 'event: end\ndata: {"reason":"renew the ticket"}\n\n'

    expect(readLiveStream(event(snapshot) + end)).toEqual({ snapshots: [snapshot], rest: '' })
  })

  it('skips a malformed event and keeps reading', () => {
    expect(readLiveStream('data: {not json\n\n' + event(snapshot)).snapshots).toEqual([snapshot])
  })
})
