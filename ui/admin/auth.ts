// Auth.js (NextAuth v5) wiring for the Aether admin UI.
//
// Two modes:
//
//   1. OIDC (production): set AUTH_OIDC_ISSUER, AUTH_OIDC_CLIENT_ID,
//      AUTH_OIDC_CLIENT_SECRET, and AUTH_SECRET. The middleware
//      protects every page; unauthenticated users are bounced to
//      the IdP's sign-in page.
//
//   2. Lab bypass (default): no OIDC env set → auth is disabled and
//      every page is reachable without a session. Shell.tsx renders
//      a yellow banner so the lab footprint is unmistakable. Lab
//      use only — never expose the UI on a network in this mode.
//
// The Shell sign-out button hits /api/auth/signout, which Auth.js
// implements automatically.

import NextAuth from 'next-auth';

export const oidcEnabled =
  !!process.env.AUTH_OIDC_ISSUER &&
  !!process.env.AUTH_OIDC_CLIENT_ID &&
  !!process.env.AUTH_OIDC_CLIENT_SECRET;

const providers = oidcEnabled
  ? [
      {
        id: 'oidc',
        name: 'OIDC',
        type: 'oidc' as const,
        issuer: process.env.AUTH_OIDC_ISSUER!,
        clientId: process.env.AUTH_OIDC_CLIENT_ID!,
        clientSecret: process.env.AUTH_OIDC_CLIENT_SECRET!,
        // Standard scopes; operators wanting groups/roles add `groups`
        // via AUTH_OIDC_SCOPES.
        authorization: {
          params: {
            scope: process.env.AUTH_OIDC_SCOPES || 'openid profile email',
          },
        },
      },
    ]
  : [];

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers,
  // Sessions live on the server (database-free); JWT is fine for the
  // operator console and avoids the persistence story.
  session: { strategy: 'jwt' },
  // Bounce unauthenticated users to /signin only when OIDC is on.
  pages: { signIn: '/signin' },
  callbacks: {
    authorized: async ({ auth: session }) => {
      // Lab bypass: when OIDC isn't configured, every request is
      // authorised. The Shell shows a banner saying so.
      if (!oidcEnabled) return true;
      return !!session;
    },
  },
  trustHost: true,
});
