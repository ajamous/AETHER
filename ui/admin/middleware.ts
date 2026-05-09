// Auth.js middleware. When OIDC is configured, every page request
// is gated on a valid session; the API auth routes (/api/auth/*) and
// the sign-in page are excluded so the OAuth dance can complete.
//
// In lab mode (no OIDC env), the `authorized` callback in auth.ts
// short-circuits to true and the middleware is a no-op.

export { auth as middleware } from '@/auth';

export const config = {
  // Match everything except Next.js internals, public files, and
  // the auth-handler routes themselves.
  matcher: ['/((?!_next/static|_next/image|favicon.ico|api/auth|signin).*)'],
};
