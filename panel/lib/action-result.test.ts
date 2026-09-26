import { describe, expect, it } from 'vitest'
import { resultMessage } from './action-result'

describe('resultMessage', () => {
  it('is the message of a success', () => {
    expect(resultMessage({ ok: true, message: 'Convite reativado' })).toBe('Convite reativado')
  })

  it('is the error of a failure', () => {
    expect(resultMessage({ ok: false, error: 'Sem permissão.' })).toBe('Sem permissão.')
  })
})
