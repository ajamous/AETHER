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
    // Capture the IdP's id_token on first sign-in so we can forward
    // it as a Bearer credential to the gateway. We deliberately do
    // NOT capture the access_token: the gateway gates on the user's
    // ID token (the IdP-signed proof of who's logged in), not on a
    // separate API access token. Aligning to id_token also keeps
    // the audit trail single-purpose — every server-to-gateway call
    // is bound to the originating session.
    //
    // The id_token is short-lived (typical IdP lifetimes 1h-24h);
    // when it expires the gateway returns 401 with reason=expired
    // and the user is bounced through Auth.js's silent re-auth or
    // back to the IdP sign-in page. We do not implement a refresh
    // dance here — the operator console is fine to re-authenticate
    // once a session.
    jwt: async ({ token, account }) => {
      if (account?.id_token) {
        (token as { idToken?: string }).idToken = account.id_token;
      }
      return token;
    },
    session: async ({ session, token }) => {
      const idToken = (token as { idToken?: string }).idToken;
      if (idToken) {
        (session as { idToken?: string }).idToken = idToken;
      }
      return session;
    },
  },
  trustHost: true,
});
