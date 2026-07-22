import { describe, expect, it } from 'vitest';
import { modelName } from './shellyModels';

describe('modelName', () => {
  it('resolves a known SKU to its marketing name', () => {
    expect(modelName('SNSW-001X16EU')).toBe('Shelly Plus 1');
  });

  it('returns undefined for an unknown or missing SKU', () => {
    expect(modelName('NOT-A-REAL-SKU')).toBeUndefined();
    expect(modelName(null)).toBeUndefined();
    expect(modelName(undefined)).toBeUndefined();
  });
});
