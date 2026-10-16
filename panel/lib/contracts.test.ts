import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest'
import actionResponse from '../../testdata/contracts/action-response.json'
import connectionToken from '../../testdata/contracts/connection-token.json'
import liveSnapshot from '../../testdata/contracts/live-snapshot.json'
import panelAudit from '../../testdata/contracts/panel-audit.json'
import panelCreateInvite from '../../testdata/contracts/panel-create-invite.json'
import panelDevices from '../../testdata/contracts/panel-devices.json'
import panelInvites from '../../testdata/contracts/panel-invites.json'
import panelMaxLeases from '../../testdata/contracts/panel-max-leases.json'
import panelUsage from '../../testdata/contracts/panel-usage.json'
import ticket from '../../testdata/contracts/ticket.json'
import type { AuditRequest } from './audit'
import { buildConnectionToken, type ConnectionTokenPayload } from './connection-token'
import type { DevicesResponse } from './devices'
import type { CreateInviteRequest, InvitesResponse, MaxLeasesRequest } from './invites'
import type { LivePeer, LiveSnapshot } from './live-snapshot'
import type { ActionResponse } from './provisiond'
import { issueTicket, type TicketPayload } from './ticket'
import type { UsageResponse } from './usage'

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

  describe('panel data routes', () => {
    type Invite = InvitesResponse['invites'][number]
    type Device = DevicesResponse['devices'][number]

    it('invites have exactly the InvitesResponse keys', () => {
      expectTypeOf(panelInvites).toExtend<InvitesResponse>()
      expectTypeOf<MissingKeys<Invite, (typeof panelInvites.invites)[number]>>().toBeNever()
      expectTypeOf<MissingKeys<(typeof panelInvites.invites)[number], Invite>>().toBeNever()
    })

    it('devices satisfy DevicesResponse', () => {
      expectTypeOf(panelDevices).toExtend<DevicesResponse>()
      expectTypeOf<MissingKeys<DevicesResponse, typeof panelDevices>>().toBeNever()
      // Optional keys are absent from the second device, so only the first is checked whole.
      expectTypeOf<MissingKeys<Device, (typeof panelDevices.devices)[0]>>().toBeNever()
    })

    it('usage has exactly the UsageResponse keys', () => {
      expectTypeOf(panelUsage).toExtend<UsageResponse>()
      expectTypeOf<
        MissingKeys<UsageResponse['days'][number], (typeof panelUsage.days)[number]>
      >().toBeNever()
    })

    it('requests have exactly the request keys', () => {
      expectTypeOf(panelCreateInvite).toExtend<CreateInviteRequest>()
      expectTypeOf<MissingKeys<CreateInviteRequest, typeof panelCreateInvite>>().toBeNever()
      expectTypeOf(panelMaxLeases).toExtend<MaxLeasesRequest>()
      expectTypeOf<MissingKeys<MaxLeasesRequest, typeof panelMaxLeases>>().toBeNever()
      expectTypeOf(panelAudit).toExtend<AuditRequest>()
      expectTypeOf<MissingKeys<AuditRequest, typeof panelAudit>>().toBeNever()
      expectTypeOf<MissingKeys<typeof panelAudit, AuditRequest>>().toBeNever()
    })
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
