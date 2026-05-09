'use client';

import { Shell } from '@/components/Shell';

export default function GlobalError({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <Shell>
      <h1 className="text-xl font-semibold mb-4">Something broke</h1>
      <p className="text-sm text-zinc-600 dark:text-zinc-300 mb-4">{error.message}</p>
      <button
        onClick={reset}
        className="px-3 py-1.5 rounded bg-accent-600 text-white text-sm hover:bg-accent-700"
      >
        Try again
      </button>
    </Shell>
  );
}
