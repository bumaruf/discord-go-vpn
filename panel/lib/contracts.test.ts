import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest'
import actionResponse from '../../testdata/contracts/action-response.json'
import connectionToken from '../../testdata/contracts/connection-token.json'
import liveSnapshot from '../../testdata/contracts/live-snapshot.json'
import ticket from '../../testdata/contracts/ticket.json'
import { buildConnectionToken, type ConnectionTokenPayload } from './connection-token'
import type { LivePeer, LiveSnapshot } from './live-snapshot'
import type { ActionResponse } from './provisiond'
import { issueTicket, type TicketPayload } from './ticket'

// The type assertions fail in `npm run typecheck`, not at runtime. toExtend is the
// `satisfies` check, which accepts a fixture that lost an optional key; MissingKeys
// catches that.
type MissingKeys<T, Fixture> = Exclude<keyof T, keyof Fixture>

describe('contract fixtures', () => {
  it('action response has exactly the ActionResponse keys', () => {
    expectTypeOf(actionResponse).toExtend<ActionResponse>()
    expectTypeOf<MissingKeys<ActionResponse, typeof actionResponse>>().toBeNever()
    expectTypeOf<MissingKeys<typeof actionResponse, ActionResponse>>().toBeNever()
  })

  it('live snapshot satisfies LiveSnapshot', () => {
    expectTypeOf(liveSnapshot).toExtend<LiveSnapshot>()
    expectTypeOf<MissingKeys<LiveSnapshot, typeof liveSnapshot>>().toBeNever()
    expectTypeOf<MissingKeys<LivePeer, (typeof liveSnapshot.peers)[number]>>().toBeNever()
  })

  describe('connection token', () => {
    it('has exactly the ConnectionTokenPayload keys', () => {
      expectTypeOf(connectionToken).toExtend<ConnectionTokenPayload>()
      expectTypeOf<MissingKeys<ConnectionTokenPayload, typeof connectionToken>>().toBeNever()
      expectTypeOf<MissingKeys<typeof connectionToken, ConnectionTokenPayload>>().toBeNever()
    })

    it('is the payload buildConnectionToken encodes', () => {
      const token = buildConnectionToken(connectionToken)
      const body = token.slice('dgv1_'.length)
      expect(JSON.parse(Buffer.from(body, 'base64url').toString('utf8'))).toEqual(connectionToken)
    })
  })

  describe('ticket', () => {
    beforeEach(() => {
      vi.useFakeTimers()
      vi.setSystemTime((ticket.exp - 5 * 60) * 1000)
      vi.stubEnv('VPN_LIVE_SECRET', 'contract-secret')
    })

    afterEach(() => {
      vi.useRealTimers()
      vi.unstubAllEnvs()
    })

    // `role` is a Role literal and the JSON import widens it to string, so these compare the keys.
    it('has the TicketPayload keys', () => {
      expectTypeOf<MissingKeys<TicketPayload, typeof ticket>>().toBeNever()
      expectTypeOf<MissingKeys<typeof ticket, TicketPayload>>().toBeNever()
    })

    it('is the payload issueTicket signs', () => {
      const [body] = issueTicket(ticket.uid, 'admin').split('.')
      expect(JSON.parse(Buffer.from(body, 'base64url').toString('utf8'))).toEqual(ticket)
    })
  })
})
