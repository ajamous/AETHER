import type { Metadata } from 'next';
import { Shell } from '@/components/Shell';
import { Card, Empty, Mono, PageHeader, Unreachable } from '@/components/ui';
import { TemplateIcon } from '@/components/icons';
import { fetchTemplates } from '@/lib/api';

export const metadata: Metadata = { title: 'Profile templates' };

export default async function TemplatesPage() {
  const data = await fetchTemplates();

  return (
    <Shell>
      <PageHeader
        title="Profile templates"
        description="YAML profile templates that profile-builder turns into Unprotected Profile Packages (UPPs)."
      />

      <Card
        title="Available templates"
        description={data ? `${data.templates.length} loaded` : undefined}
        footer={
          <>
            Templates ship from <code>services/profile-builder/templates</code>. Add a YAML file
            there to publish a new one.
          </>
        }
        flush
      >
        {!data ? (
          <Unreachable
            service="profile-builder"
            hint="The console reaches it through the gateway."
          />
        ) : data.templates.length === 0 ? (
          <Empty
            title="No templates loaded"
            hint="profile-builder found no YAML templates in its templates directory."
          />
        ) : (
          <ul className="grid gap-px bg-zinc-100 sm:grid-cols-2 dark:bg-zinc-800/70">
            {data.templates.map((t) => (
              <li key={t} className="flex items-center gap-3 bg-white px-5 py-3.5 dark:bg-ink-900">
                <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-accent-50 text-accent-600 dark:bg-accent-500/10 dark:text-accent-400">
                  <TemplateIcon className="size-4" />
                </span>
                <Mono className="truncate">{t}</Mono>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card className="mt-6" title="Building a UPP from a template">
        <p className="text-sm leading-relaxed text-zinc-600 dark:text-zinc-300">
          Send subscriber data to <Mono>POST /v1/templates/&#123;name&#125;/build</Mono> on
          profile-builder. It returns a JSON envelope whose <Mono>saip_der</Mono> field carries the
          DER-encoded SAIP profile package. Today that covers the ProfileHeader and PE-End; richer
          ProfileElements arrive as <Mono>pkg/saip</Mono> grows. The gateway proxies the read-only
          template endpoints, not the build call.
        </p>
      </Card>
    </Shell>
  );
}
