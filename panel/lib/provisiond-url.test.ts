import { afterEach, describe, expect, it, vi } from 'vitest'
import { provisiondUrl } from './provisiond-url'

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('provisiondUrl', () => {
  it.each([undefined, '', '   '])('is undefined when PROVISIOND_URL is %j', (value) => {
    vi.stubEnv('PROVISIOND_URL', value)
    expect(provisiondUrl('/admin/live')).toBeUndefined()
  })

  it.each(['https://vpn.example.com', 'https://vpn.example.com/'])(
    'joins the path to %s',
    (base) => {
      vi.stubEnv('PROVISIOND_URL', base)
      expect(provisiondUrl('/admin/live')).toBe('https://vpn.example.com/admin/live')
    },
  )
})
