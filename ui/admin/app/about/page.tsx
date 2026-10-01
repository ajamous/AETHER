import type { Metadata } from 'next';
import { Shell } from '@/components/Shell';
import { Card, Mono, NotYet, PageHeader } from '@/components/ui';
import { oidcEnabled } from '@/auth';

export const metadata: Metadata = { title: 'About' };

export default function AboutPage() {
  return (
    <Shell>
      <PageHeader
        title="About this console"
        description="The operator surface for the Aether remote SIM provisioning stack."
      />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="What it does">
          <p className="text-sm leading-relaxed text-zinc-600 dark:text-zinc-300">
            A read-only view across the stack: service health, identity certificates, profile
            templates, SM-DS discovery events, eIM IoT devices, and the hash-chained audit log. All
            data is fetched server-side; the browser never talks to the backend services directly.
          </p>
        </Card>

        <Card title="Authentication">
          <p className="text-sm leading-relaxed text-zinc-600 dark:text-zinc-300">
            {oidcEnabled ? (
              <>
                OIDC sign-in is enabled. Your session&apos;s ID token is forwarded to the gateway,
                which checks it on <Mono>/v1/*</Mono> admin routes when the gateway&apos;s own OIDC
                gate is switched on.
              </>
            ) : (
              <>
                Authentication is <strong className="font-semibold">disabled</strong> because no
                OIDC provider is configured (lab mode). Set <Mono>AUTH_OIDC_ISSUER</Mono>,{' '}
                <Mono>AUTH_OIDC_CLIENT_ID</Mono>, <Mono>AUTH_OIDC_CLIENT_SECRET</Mono> and{' '}
                <Mono>AUTH_SECRET</Mono> to require sign-in.
              </>
            )}
          </p>
        </Card>
      </div>

      <div className="mt-6">
        <NotYet
          items={[
            'Write actions: profile activation, certificate rotation, HSM administration',
            'Live updates (reload the page to see new data)',
            'Accessibility audit (target: WCAG 2.1 AA)',
          ]}
        />
      </div>

      <p className="mt-6 text-sm text-zinc-500 dark:text-zinc-400">
        Source: <Mono>ui/admin/</Mono> in the Aether repository. Each service&apos;s README
        documents the endpoints it exposes.
      </p>
    </Shell>
  );
}
