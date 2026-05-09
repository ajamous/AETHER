import { Shell } from '@/components/Shell';
import { Card } from '@/components/Card';

export default function AboutPage() {
  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-6">About this UI</h1>

      <Card title="What this is">
        <p className="text-sm text-zinc-600 dark:text-zinc-300">
          The Aether admin console is the operator surface for the Aether RSP stack.
          The current build covers system health, profile templates, certificates,
          and the audit log. It is read-only today.
        </p>
      </Card>

      <Card title="What it doesn't do yet">
        <ul className="text-sm list-disc list-inside text-zinc-600 dark:text-zinc-300 space-y-1">
          <li>No authentication. Lab use only — never expose to a network.</li>
          <li>No write actions. Manual provisioning, cert rotation, and HSM admin land in follow-ups.</li>
          <li>No real-time updates yet (refresh to see new entries).</li>
          <li>No accessibility audit yet (target: WCAG 2.1 AA).</li>
        </ul>
      </Card>

      <Card title="Source">
        <p className="text-sm">
          See <code className="font-mono">ui/admin/</code> in the repository, and the
          per-service READMEs for what each backend exposes.
        </p>
      </Card>
    </Shell>
  );
}
