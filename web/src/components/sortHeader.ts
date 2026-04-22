export type SortDir = 'asc' | 'desc';

export interface SortHeaderState {
  active: boolean;
  ariaSort: 'ascending' | 'descending' | 'none';
  nextDir: 'ascending' | 'descending';
  indicator: string;
}

export function deriveSortHeaderState(
  sortKey: string,
  column: string,
  sortDir: SortDir,
): SortHeaderState {
  const active = sortKey === column;
  if (!active) {
    return { active: false, ariaSort: 'none', nextDir: 'ascending', indicator: '' };
  }
  return {
    active: true,
    ariaSort: sortDir === 'asc' ? 'ascending' : 'descending',
    nextDir: sortDir === 'asc' ? 'descending' : 'ascending',
    indicator: sortDir === 'asc' ? ' ▲' : ' ▼',
  };
}
