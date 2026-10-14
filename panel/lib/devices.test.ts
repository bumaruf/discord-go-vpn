import { describe, expect, it, vi } from 'vitest'
import devicesFixture from '../../testdata/contracts/panel-devices.json'
import { listDevices } from './devices'
import { readProvisiond } from './provisiond'

vi.mock('./provisiond', () => ({ readProvisiond: vi.fn() }))

describe('listDevices', () => {
  it('maps every machine and keeps the invite labels', async () => {
    vi.mocked(readProvisiond).mockResolvedValue(devicesFixture)

    expect(await listDevices()).toEqual({
      devices: [
        {
          id: '66a1b2c3d4e5f6a7b8c9d0e1',
          inviteId: '65f1a2b3c4d5e6f7a8b9c0d1',
          name: 'DESKTOP-ANA',
          osUser: 'ana',
          lastIp: '203.0.113.7',
          firstSeenAt: '2026-09-01T10:00:00Z',
          lastSeenAt: '2026-09-12T11:59:00Z',
          accessCount: 42,
        },
        {
          id: '66a1b2c3d4e5f6a7b8c9d0e2',
          inviteId: undefined,
          name: '',
          osUser: undefined,
          lastIp: undefined,
          firstSeenAt: '2026-08-20T10:00:00Z',
          lastSeenAt: '2026-08-21T10:00:00Z',
          accessCount: 1,
        },
      ],
      inviteLabels: { '65f1a2b3c4d5e6f7a8b9c0d1': 'Família' },
    })
    expect(readProvisiond).toHaveBeenCalledWith('/admin/devices')
  })
})
