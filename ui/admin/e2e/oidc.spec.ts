import { expect, test } from '@playwright/test';
import { clientId, issuer } from '../playwright.config';

// Full OIDC round trip: an unauthenticated visit is sent to /signin,
// the IdP authenticates the operator, the console shows them signed in
// and forwards their id_token to the gateway, and sign-out ends the
// session.

const operator = { name: 'Ada Operator', email: 'ada@example.org' };

function decodeJwtPayload(jwt: string): Record<string, unknown> {
  const payload = jwt.split('.')[1];
  return JSON.parse(Buffer.from(payload, 'base64url').toString('utf8'));
}

test('operator signs in through the IdP, is forwarded to the gateway, and signs out', async ({
  page,
  request,
}) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/signin\?callbackUrl=/);

  await page.getByRole('button', { name: 'Sign in with OIDC' }).click();
  await expect(page).toHaveURL(new RegExp(`^${issuer}/authorize\\?`));
  // Authorization-code flow with PKCE.
  expect(new URL(page.url()).searchParams.get('code_challenge_method')).toBe('S256');

  // mock-oauth2-server's login form: subject + optional claims JSON.
  await page.fill('input[name=username]', 'ada');
  await page.fill('textarea[name=claims]', JSON.stringify(operator));
  await page.click('input[type=submit]');

  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();
  const sidebar = page.locator('aside');
  await expect(sidebar).toContainText(operator.name);
  await expect(sidebar).toContainText(operator.email);
  await expect(page.getByRole('region', { name: 'Lab mode warning' })).toHaveCount(0);

  // The dashboard's server-side fetch to the gateway carried the
  // session's id_token (lib/api.ts gatewayAuthHeaders).
  const seen: { path: string; authorization: string | null }[] = await (
    await request.get('http://localhost:8080/__seen')
  ).json();
  const health = seen.filter((s) => s.path === '/v1/health' && s.authorization);
  expect(health.length).toBeGreaterThan(0);
  const auth = health[health.length - 1].authorization!;
  expect(auth).toMatch(/^Bearer [\w-]+\.[\w-]+\.[\w-]+$/);
  const claims = decodeJwtPayload(auth.slice('Bearer '.length));
  expect(claims.iss).toBe(issuer);
  expect(claims.sub).toBe('ada');
  expect([claims.aud].flat()).toContain(clientId);

  // Ordinary page traffic (here, an RSC request like a link prefetch)
  // must not re-issue the session cookie: a response to a request in
  // flight during sign-out would otherwise set it again after sign-out
  // cleared it (middleware.ts).
  const prefetch = await page.request.get('/templates', { headers: { RSC: '1' } });
  expect(prefetch.ok()).toBe(true);
  const reissued = prefetch
    .headersArray()
    .filter((h) => h.name.toLowerCase() === 'set-cookie' && /authjs\.session-token/.test(h.value));
  expect(reissued).toEqual([]);

  // Sign out straight away, while the dashboard's link prefetches may
  // still be in flight.
  await sidebar.getByRole('button', { name: 'Sign out' }).click();
  await expect(page).toHaveURL(/\/signin$/);

  // The session is gone: protected pages bounce back to sign-in.
  await page.goto('/certs');
  await expect(page).toHaveURL(/\/signin\?callbackUrl=/);
});
