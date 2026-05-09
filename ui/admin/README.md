# ui/admin

The Aether operator console. Next.js 14 (App Router), React 18, Tailwind CSS.

## Status

| Page                  | Status        | Notes                                              |
| --------------------- | ------------- | -------------------------------------------------- |
| Dashboard             | Implemented   | System health, cert expiry summary, audit chain status |
| Profile templates     | Implemented (read-only) | Lists templates from profile-builder via gateway |
| Certificates          | Implemented (read-only) | Identity certs with subject/issuer/expiry       |
| Audit log             | Implemented (read-only) | Last 50 entries with chain integrity status     |
| About                 | Implemented   |                                                    |
| OIDC sign-in          | Implemented (Auth.js v5) | Bypass with banner when unconfigured (lab default) |
| Real-time updates     | Not started   | Refresh-to-update for now                          |
| Profile activation flow | Not started |                                                    |
| Cert rotation         | Not started   |                                                    |
| HSM admin             | Not started   |                                                    |
| Storybook             | Not started   |                                                    |
| Accessibility audit   | Not started   | Target: WCAG 2.1 AA                                |

The console is read-only today. **In lab mode it has no
authentication** — the Shell renders an unmissable yellow
"AUTH DISABLED" banner so the running state is obvious. To
enable OIDC, set the four env vars listed under "OIDC
configuration" below; auth becomes mandatory and unauthenticated
requests are bounced to the IdP's sign-in page.

## OIDC configuration

Auth.js v5 handles the OAuth flow. Four env vars enable it:

| Variable                  | Purpose                                              |
| ------------------------- | ---------------------------------------------------- |
| `AUTH_OIDC_ISSUER`        | OpenID Connect issuer URL (e.g. `https://idp.example/realms/aether`) |
| `AUTH_OIDC_CLIENT_ID`     | OAuth client ID issued by your IdP                   |
| `AUTH_OIDC_CLIENT_SECRET` | OAuth client secret                                  |
| `AUTH_SECRET`             | Random 32-byte string for cookie signing — `openssl rand -base64 32` |

Optional:

| Variable             | Purpose                                                      |
| -------------------- | ------------------------------------------------------------ |
| `AUTH_OIDC_SCOPES`   | OAuth scopes (default `openid profile email`)                |
| `AUTH_URL`           | The canonical public URL of the UI as the IdP sees it; required when behind an ingress so the redirect URI is correct |

The Helm chart wires these via the `ui.oidc.*` block — see
[deployments/helm/aether/values.yaml](https://github.com/ajamous/aether/blob/main/deployments/helm/aether/values.yaml).

Sign-out is a button in the sidebar that POSTs to
`/api/auth/signout`. The signed-out user's email shows in the
sidebar, so the operator always knows who they are.

## Running

```
cd ui/admin
npm install
npm run dev      # http://localhost:3000
```

The default backend URLs assume the lab Docker Compose stack:

- Gateway: `http://localhost:8080`
- Audit: `http://localhost:8447`
- Certmgr: `http://localhost:8444`

Override with `AETHER_GATEWAY_URL`, `AETHER_AUDIT_URL`, `AETHER_CERTMGR_URL`.

## Build

```
npm run build
npm run start
```

The Next.js `output: 'standalone'` build produces a self-contained
runtime under `.next/standalone/` suitable for the Dockerfile.

## Tests

`npm run typecheck` and `npm run lint` are gated in CI. There are no
component tests yet; we add Vitest + Testing Library when the first
non-trivial interactive component lands (search box, profile editor).

## Notes on data fetching

All upstream calls go through `lib/api.ts` server-side. The browser
never sees backend URLs or talks to the services directly. When
authentication lands, the OIDC token also flows server-side; the
client gets sanitized payloads only.
