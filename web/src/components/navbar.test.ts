import { describe, expect, it } from 'vitest';
import { isNavLinkActive, normalizeNavVersion } from './navbar';

describe('normalizeNavVersion', () => {
  it('strips a single leading v so the badge does not render "vv..."', () => {
    expect(normalizeNavVersion('v0.3.5')).toBe('0.3.5');
  });

  it('leaves a version without a leading v unchanged', () => {
    expect(normalizeNavVersion('0.3.5')).toBe('0.3.5');
  });

  it('only strips the first v (git-describe suffix preserved)', () => {
    expect(normalizeNavVersion('v0.3.5-3-gf0ea960')).toBe('0.3.5-3-gf0ea960');
  });

  it('handles an empty string', () => {
    expect(normalizeNavVersion('')).toBe('');
  });
});

describe('isNavLinkActive', () => {
  it('marks a link active on an exact path match', () => {
    expect(isNavLinkActive('/settings', '/settings')).toBe(true);
  });

  it('does not mark a link active for a different path', () => {
    expect(isNavLinkActive('/settings', '/logs')).toBe(false);
  });

  it('keeps the Devices link ("/") active on device-detail routes', () => {
    expect(isNavLinkActive('/', '/devices/AABBCCDDEEFF')).toBe(true);
  });

  it('does not bleed the device-detail rule onto other links', () => {
    expect(isNavLinkActive('/scan', '/devices/AABBCCDDEEFF')).toBe(false);
  });

  it('matches the Devices link on the root path exactly', () => {
    expect(isNavLinkActive('/', '/')).toBe(true);
  });
});
