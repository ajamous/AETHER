import { Shell } from '@/components/Shell';

export default function NotFound() {
  return (
    <Shell>
      <h1 className="text-xl font-semibold mb-4">Not found</h1>
      <p className="text-sm text-zinc-600 dark:text-zinc-300">
        That page doesn&apos;t exist. Use the sidebar.
      </p>
    </Shell>
  );
}
