// Thin server-side fetch helpers for the Aether admin UI.
//
// Server components call these directly; we never expose API keys
// to the browser. When OIDC lands, the auth header travels through
// the same helpers via `next/headers`.

const GATEWAY = process.env.AETHER_GATEWAY_URL || 'http://localhost:8080';
const AUDIT = process.env.AETHER_AUDIT_URL || 'http://localhost:8447';
const CERTMGR = process.env.AETHER_CERTMGR_URL || 'http://localhost:8444';

type FetchOpts = {
  cache?: RequestCache;
  revalidate?: number | false;
};

async function get<T>(url: string, opts: FetchOpts = {}): Promise<T | null> {
  try {
    const res = await fetch(url, {
      cache: opts.cache ?? 'no-store',
      next: opts.revalidate !== undefined ? { revalidate: opts.revalidate as number } : undefined,
    });
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

// ---- Gateway ---------------------------------------------------------------

export type GatewayHealth = {
  ready: boolean;
  upstream: Record<string, string>;
};
export const fetchGatewayHealth = () => get<GatewayHealth>(`${GATEWAY}/v1/health`);

export type Templates = { templates: string[] };
export const fetchTemplates = () => get<Templates>(`${GATEWAY}/v1/templates`);

// ---- Certmgr ---------------------------------------------------------------

export type CertView = {
  name: string;
  subject: string;
  issuer: string;
  not_before: string;
  not_after: string;
  days_until_expiry: number;
  serial_number: string;
  loaded_at: string;
};
export const fetchCerts = () => get<CertView[]>(`${CERTMGR}/v1/certs`);

export type CertHealth = {
  ready: boolean;
  mode: 'lab' | 'production';
  identities: number;
  trust_store_size: number;
  expiring_soon: string[];
  earliest_expiry_days: number;
};
export const fetchCertmgrHealth = () => get<CertHealth>(`${CERTMGR}/v1/health`);

// ---- Audit -----------------------------------------------------------------

export type AuditEntry = {
  seq: number;
  timestamp: string;
  payload: unknown;
  prev_hash: string;
  hash: string;
};
export type AuditList = { length: number; entries: AuditEntry[] };
export const fetchAuditEntries = (since = 0) =>
  get<AuditList>(`${AUDIT}/v1/events?since=${since}`);

export type VerifyResult = {
  ok: boolean;
  length: number;
  failed_at_seq?: number;
  reason?: string;
};
export const fetchAuditVerify = () => get<VerifyResult>(`${AUDIT}/v1/verify`);
