import { describe, expect, it } from 'vitest'
import { deriveSortHeaderState } from './sortHeader'

describe('deriveSortHeaderState', () => {
  it('returns inactive state when column is not the sort key', () => {
    expect(deriveSortHeaderState('name', 'ip', 'asc')).toEqual({
      active: false,
      ariaSort: 'none',
      nextDir: 'ascending',
      indicator: '',
    })
  })

  it('returns ascending indicators when active and sorted asc', () => {
    expect(deriveSortHeaderState('name', 'name', 'asc')).toEqual({
      active: true,
      ariaSort: 'ascending',
      nextDir: 'descending',
      indicator: ' ▲',
    })
  })

  it('returns descending indicators when active and sorted desc', () => {
    expect(deriveSortHeaderState('name', 'name', 'desc')).toEqual({
      active: true,
      ariaSort: 'descending',
      nextDir: 'ascending',
      indicator: ' ▼',
    })
  })

  it('toggling sort direction inverts ariaSort and nextDir', () => {
    const asc = deriveSortHeaderState('ip', 'ip', 'asc')
    const desc = deriveSortHeaderState('ip', 'ip', 'desc')
    expect(asc.ariaSort).toBe('ascending')
    expect(desc.ariaSort).toBe('descending')
    expect(asc.nextDir).toBe('descending')
    expect(desc.nextDir).toBe('ascending')
  })
})
