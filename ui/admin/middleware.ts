// Auth.js middleware. When OIDC is configured, every page request
// is gated on a valid session; the API auth routes (/api/auth/*) and
// the sign-in page are excluded so the OAuth dance can complete.
//
// In lab mode (no OIDC env), the `authorized` callback in auth.ts
// short-circuits to true and the middleware is a no-op.

import type { NextFetchEvent, NextRequest } from 'next/server';
import { auth } from '@/auth';

const authMiddleware = auth as unknown as (
  req: NextRequest,
  ev: NextFetchEvent,
) => Promise<Response | undefined>;

// authjs.session-token, its __Secure- form on HTTPS, and its .0/.1
// chunks when the JWT is large.
const SESSION_COOKIE = /^(__Secure-)?authjs\.session-token(\.\d+)?=/;

export async function middleware(req: NextRequest, ev: NextFetchEvent) {
  const res = await authMiddleware(req, ev);
  if (!res) return res;

  // Auth.js re-issues the session cookie on every request it sees,
  // including Next.js link prefetches. A prefetch still in flight when
  // the operator signs out then lands after the sign-out response and
  // sets the session cookie again, so "Sign out" silently fails.
  // Drop that re-issue here: only the sign-in callback and sign-out
  // (both outside this middleware's effect) write the cookie. The cost
  // is a fixed session lifetime from sign-in instead of a sliding one.
  const cookies = res.headers.getSetCookie();
  if (cookies.some((c) => SESSION_COOKIE.test(c))) {
    res.headers.delete('set-cookie');
    for (const c of cookies) {
      if (!SESSION_COOKIE.test(c)) res.headers.append('set-cookie', c);
    }
  }
  return res;
}

export const config = {
  // Match everything except Next.js internals, public files, and
  // the auth-handler routes themselves.
  matcher: ['/((?!_next/static|_next/image|favicon.ico|api/auth|signin).*)'],
};
