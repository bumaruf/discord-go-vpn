import { describe, expect, it } from 'vitest'
import { parseMaxLeases } from './max-leases'

describe('parseMaxLeases', () => {
  it.each([
    ['', null],
    ['  ', null],
    [null, null],
    ['1', 1],
    [' 4 ', 4],
    ['100', 100],
    ['0', undefined],
    ['101', undefined],
    ['-2', undefined],
    ['2.5', undefined],
    ['quatro', undefined],
  ])('reads %j as %j', (raw, expected) => {
    expect(parseMaxLeases(raw)).toBe(expected)
  })
})
