import { describe, expect, it } from 'vitest'
import { deviceName } from './device-name'

describe('deviceName', () => {
  it('shows the hostname and the OS user', () => {
    expect(deviceName({ name: 'notebook da ana', osUser: 'ana' })).toBe('notebook da ana · ana')
  })

  it.each([undefined, ''])('shows only the hostname when the OS user is %j', (osUser) => {
    expect(deviceName({ name: 'notebook da ana', osUser })).toBe('notebook da ana')
  })

  it('falls back to "sem nome" without a hostname', () => {
    expect(deviceName({ name: '' })).toBe('sem nome')
    expect(deviceName({ name: '', osUser: 'ana' })).toBe('sem nome · ana')
  })
})
