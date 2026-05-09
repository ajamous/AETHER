// Thin server-side fetch helpers for the Aether admin UI.
//
// Server components call these directly; we never expose API keys
// to the browser.
//
// When OIDC is configured, the user's session carries the IdP's
// id_token (captured by the auth.ts JWT callback). We forward it
// as `Authorization: Bearer ${idToken}` ONLY on calls to the
// gateway — the gateway is the OIDC enforcer, and AUDIT/CERTMGR
// are reached over the cluster network without their own gate.
// In lab mode (no OIDC), no header is added and the gateway's
// /v1/* paths are unauthenticated.

import { auth } from '@/auth';

const GATEWAY = process.env.AETHER_GATEWAY_URL || 'http://localhost:8080';
const AUDIT = process.env.AETHER_AUDIT_URL || 'http://localhost:8447';
const CERTMGR = process.env.AETHER_CERTMGR_URL || 'http://localhost:8444';

type FetchOpts = {
  cache?: RequestCache;
  revalidate?: number | false;
};

// gatewayAuthHeaders returns an Authorization header for outbound
// requests to the gateway when a session id_token is available.
// Returns an empty object otherwise — both in lab (no OIDC) and
// when the user is unauthenticated. The empty-object form keeps
// the call site simple: spread the result into the headers init.
async function gatewayAuthHeaders(url: string): Promise<Record<string, string>> {
  if (!url.startsWith(GATEWAY)) {
    // AUDIT and CERTMGR have no OIDC gate; forwarding a Bearer
    // would leak the operator's id_token to services that don't
    // need it.
    return {};
  }
  try {
    const session = (await auth()) as { idToken?: string } | null;
    const idToken = session?.idToken;
    if (idToken) {
      return { Authorization: `Bearer ${idToken}` };
    }
  } catch {
    // auth() can throw outside a Next.js request scope; fall
    // through to the no-header path so server-side rendering
    // doesn't crash on misconfigurations.
  }
  return {};
}

async function get<T>(url: string, opts: FetchOpts = {}): Promise<T | null> {
  try {
    const headers = await gatewayAuthHeaders(url);
    const res = await fetch(url, {
      cache: opts.cache ?? 'no-store',
      next: opts.revalidate !== undefined ? { revalidate: opts.revalidate as number } : undefined,
      headers,
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

// ---- SM-DS -----------------------------------------------------------------

export type SMDSEvent = {
  eid: string;
  event_id: string;
  rsp_server_address: string;
  forwarding: boolean;
  registered_at: string;
};
export type SMDSEvents = { length: number; events: SMDSEvent[] };
export const fetchSMDSEvents = () => get<SMDSEvents>(`${GATEWAY}/v1/smds/events`);

// ---- eIM (SGP.32) ----------------------------------------------------------

export type IoTDevice = {
  eid: string;
  label?: string;
  tags?: string[];
  registered_at: string;
  last_seen?: string | null;
};
export type IoTDevices = { length: number; devices: IoTDevice[] };
export const fetchIoTDevices = () => get<IoTDevices>(`${GATEWAY}/v1/eim/devices`);
