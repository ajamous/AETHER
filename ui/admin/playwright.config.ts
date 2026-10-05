// End-to-end tests for the console against a real OIDC provider.
//
// Needs an OIDC issuer at OIDC_ISSUER (default http://localhost:8099/aether).
// CI runs navikt/mock-oauth2-server as a service container; locally:
//
//   docker run -d -p 8099:8080 ghcr.io/navikt/mock-oauth2-server:2.1.10
//   npm run build && npm run test:e2e
//
// The console is started in OIDC mode by `webServer` below, next to a
// fake gateway that records the Bearer header it receives.

import { defineConfig } from '@playwright/test';

export const issuer = process.env.OIDC_ISSUER || 'http://localhost:8099/aether';
export const clientId = 'aether-console';

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  retries: 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    // Everything on "localhost": the console's callback URL, its cookies
    // and the IdP must agree on the host.
    baseURL: 'http://localhost:3000',
    trace: 'retain-on-failure',
    // Optional: use a preinstalled Chromium instead of `playwright install`.
    launchOptions: process.env.PW_CHROMIUM_PATH
      ? { executablePath: process.env.PW_CHROMIUM_PATH }
      : {},
  },
  webServer: [
    {
      command: 'node e2e/fake-gateway.mjs',
      port: 8080,
      reuseExistingServer: false,
    },
    {
      // Requires a prior `npm run build`.
      command: 'npx next start -p 3000',
      url: 'http://localhost:3000/signin',
      reuseExistingServer: false,
      env: {
        AUTH_OIDC_ISSUER: issuer,
        AUTH_OIDC_CLIENT_ID: clientId,
        AUTH_OIDC_CLIENT_SECRET: 'e2e-secret',
        AUTH_SECRET: 'e2e-only-secret-0123456789abcdef0123',
        AUTH_URL: 'http://localhost:3000',
      },
    },
  ],
});
