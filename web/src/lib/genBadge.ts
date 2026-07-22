import type { AppSettings } from './types';

export function genLabel(gen: number, frozen = false): string {
  return frozen ? `Gen ${gen}.x (frozen)` : `Gen ${gen}.x`;
}

export function genTitle(gen: number, frozen = false): string {
  if (frozen) {
    return 'Feature-frozen — will never receive 2.0.0+ (Shelly Firmware Update Policy)';
  }
  if (gen === 2) return 'Limited support';
  return 'Supported';
}

export function genBadgeClass(gen: number, settings?: AppSettings | null, frozen = false): string {
  if (frozen) return settings?.gen_frozen_badge_class || 'bg-warning text-dark';
  if (gen === 2) return settings?.gen2_badge_class || 'bg-warning text-dark';
  if (gen >= 4) return settings?.gen4_badge_class || 'bg-info text-dark';
  return settings?.gen3_badge_class || 'bg-success';
}
