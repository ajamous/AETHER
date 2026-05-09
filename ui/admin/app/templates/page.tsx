import { Shell } from '@/components/Shell';
import { Card, Empty } from '@/components/Card';
import { fetchTemplates } from '@/lib/api';

export default async function TemplatesPage() {
  const data = await fetchTemplates();

  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-6">Profile templates</h1>
      <Card
        title="Available templates"
        footer="Templates ship from services/profile-builder/templates. Add new YAML files there."
      >
        {!data ? (
          <Empty message="profile-builder unreachable via gateway" />
        ) : data.templates.length === 0 ? (
          <Empty message="No templates loaded yet." />
        ) : (
          <ul className="text-sm font-mono space-y-1">
            {data.templates.map((t) => (
              <li key={t} className="border-b border-zinc-100 dark:border-zinc-800 py-1.5">
                {t}
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card title="Building a UPP from a template" footer="POST /v1/templates/{name}/build via gateway">
        <p className="text-sm text-zinc-600 dark:text-zinc-300">
          The build endpoint accepts subscriber data and returns a UPP envelope. Today
          the envelope is JSON-shaped; the SAIP-encoded UPP takes its place once
          <code className="font-mono mx-1">pkg/saip</code>
          lands. The endpoint signature does not change.
        </p>
      </Card>
    </Shell>
  );
}
