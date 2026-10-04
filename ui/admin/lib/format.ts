// Presentation helpers shared by the server-rendered pages. Pure
// functions only — no fetches, no React.

export type Tone = 'ok' | 'warn' | 'danger' | 'neutral' | 'info';

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
];

// "12 minutes ago", "in 3 days", "just now". Pages are rendered per
// request (all fetches are no-store), so server time is the right
// reference point.
export function relativeTime(iso: string, now: number = Date.now()): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  const seconds = Math.round((t - now) / 1000);
  for (const [unit, size] of UNITS) {
    if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit);
  }
  return 'just now';
}

// "2026-10-01 05:14:30 UTC" — unambiguous, for titles and tooltips.
export function absoluteTime(iso: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  return new Date(t)
    .toISOString()
    .replace('T', ' ')
    .replace(/\.\d+Z$/, ' UTC');
}

// Certificate expiry thresholds match the observability bundle's
// AetherCertExpiringSoon (< 30d), AetherCertExpiringUrgent (< 7d) and
// AetherCertExpired (<= 0d) alerts.
export function expiryTone(days: number): Tone {
  if (days < 7) return 'danger';
  if (days < 30) return 'warn';
  return 'ok';
}

export function expiryLabel(days: number): string {
  if (days < 0) return `expired ${-days}d ago`;
  if (days === 0) return 'expired';
  return `${days}d left`;
}

// Pull the CN out of an RFC 4514-ish DN ("CN=x,O=y,C=z"); fall back
// to the whole string.
export function commonName(dn: string): string {
  const m = /(?:^|,)\s*CN=([^,]+)/i.exec(dn);
  return m ? m[1] : dn;
}

export function shortHash(hash: string, n = 12): string {
  return hash.length > n ? `${hash.slice(0, n)}…` : hash;
}

// eIM devices: how recently the IPA polled.
export function lastSeenTone(iso: string | null | undefined, now: number = Date.now()): Tone {
  if (!iso) return 'neutral';
  const age = now - Date.parse(iso);
  if (age < 15 * 60_000) return 'ok';
  if (age < 24 * 3600_000) return 'info';
  return 'warn';
}

// Audit payloads are free-form JSON. Most carry a `kind` (or `type` /
// `event`) discriminator; surface it as the headline and the rest as
// key=value pairs.
export function summarizePayload(payload: unknown): { kind: string | null; detail: string } {
  if (payload === null || typeof payload !== 'object' || Array.isArray(payload)) {
    return { kind: null, detail: JSON.stringify(payload) };
  }
  const obj = payload as Record<string, unknown>;
  const kindKey = ['kind', 'type', 'event', 'action'].find((k) => typeof obj[k] === 'string');
  const kind = kindKey ? (obj[kindKey] as string) : null;
  const detail = Object.entries(obj)
    .filter(([k]) => k !== kindKey)
    .map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join('  ');
  return { kind, detail };
}
