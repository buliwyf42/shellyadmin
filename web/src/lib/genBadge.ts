import type { AppSettings } from './types';

export function genLabel(gen: number): string {
  return `Gen ${gen}.x`;
}

export function genTitle(gen: number): string {
  if (gen === 2) return 'Limited support';
  return 'Supported';
}

export function genBadgeClass(gen: number, settings?: AppSettings | null): string {
  if (gen === 2) return settings?.gen2_badge_class || 'bg-warning text-dark';
  if (gen >= 4) return settings?.gen4_badge_class || 'bg-info text-dark';
  return settings?.gen3_badge_class || 'bg-success';
}
