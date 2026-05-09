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
| OIDC sign-in          | Not started   | Lab build runs without auth                        |
| Real-time updates     | Not started   | Refresh-to-update for now                          |
| Profile activation flow | Not started |                                                    |
| Cert rotation         | Not started   |                                                    |
| HSM admin             | Not started   |                                                    |
| Storybook             | Not started   |                                                    |
| Accessibility audit   | Not started   | Target: WCAG 2.1 AA                                |

This is a read-only console today. **It has no authentication.** Run it
only against a lab stack on localhost. Do not expose to a network.

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
