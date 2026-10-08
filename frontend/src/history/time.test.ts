import { describe, it, expect } from 'vitest';
import { formatRelativeTime } from './time';

describe('formatRelativeTime', () => {
  const baseTime = new Date('2026-10-08T12:00:00Z');

  it('handles invalid dates gracefully', () => {
    expect(formatRelativeTime('invalid-date-string')).toBe('Unknown time');
  });

  it('formats recent events as Just now', () => {
    const recent = new Date(baseTime.getTime() - 25 * 1000).toISOString();
    expect(formatRelativeTime(recent, baseTime)).toBe('Just now');
  });

  it('formats single minute ago', () => {
    const oneMinAgo = new Date(baseTime.getTime() - 65 * 1000).toISOString();
    expect(formatRelativeTime(oneMinAgo, baseTime)).toBe('1 minute ago');
  });

  it('formats multiple minutes ago', () => {
    const fiveMinAgo = new Date(baseTime.getTime() - 5 * 60 * 1000).toISOString();
    expect(formatRelativeTime(fiveMinAgo, baseTime)).toBe('5 minutes ago');
  });

  it('formats single hour ago', () => {
    const oneHourAgo = new Date(baseTime.getTime() - 65 * 60 * 1000).toISOString();
    expect(formatRelativeTime(oneHourAgo, baseTime)).toBe('1 hour ago');
  });

  it('formats multiple hours ago', () => {
    const threeHoursAgo = new Date(baseTime.getTime() - 3 * 3600 * 1000).toISOString();
    expect(formatRelativeTime(threeHoursAgo, baseTime)).toBe('3 hours ago');
  });

  it('formats Yesterday', () => {
    const yesterday = new Date(baseTime.getTime() - 26 * 3600 * 1000).toISOString();
    expect(formatRelativeTime(yesterday, baseTime)).toBe('Yesterday');
  });

  it('formats multiple days ago', () => {
    const fiveDaysAgo = new Date(baseTime.getTime() - 5 * 24 * 3600 * 1000).toISOString();
    expect(formatRelativeTime(fiveDaysAgo, baseTime)).toBe('5 days ago');
  });

  it('formats older dates as YYYY-MM-DD', () => {
    const longAgo = new Date('2026-01-15T08:30:00Z').toISOString();
    expect(formatRelativeTime(longAgo, baseTime)).toBe('2026-01-15');
  });
});
