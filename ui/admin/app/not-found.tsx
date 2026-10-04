import { Shell } from '@/components/Shell';
import { MoreLink } from '@/components/ui';

export default function NotFound() {
  return (
    <Shell>
      <div className="py-16 text-center">
        <p className="font-mono text-sm text-accent-600 dark:text-accent-400">404</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">Page not found</h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
          That page doesn&apos;t exist. Pick a section from the navigation.
        </p>
        <div className="mt-6">
          <MoreLink href="/">Back to dashboard</MoreLink>
        </div>
      </div>
    </Shell>
  );
}
