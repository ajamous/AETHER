'use client';

// Minimal error fallback. Deliberately doesn't import Shell — Shell
// is an async server component with server-action sign-out forms,
// which can't render from inside a client component (which the
// error boundary must be).

import Link from 'next/link';
import { AlertIcon } from '@/components/icons';

export default function GlobalError({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-lg rounded-xl border border-zinc-200 bg-white p-8 shadow-xs dark:border-zinc-800 dark:bg-ink-900">
        <span className="grid size-10 place-items-center rounded-full bg-red-50 text-red-600 dark:bg-red-500/10 dark:text-red-400">
          <AlertIcon className="size-5" />
        </span>
        <h1 className="mt-4 text-lg font-semibold">Something went wrong</h1>
        <p className="mt-1 break-words text-sm text-zinc-600 dark:text-zinc-300">{error.message}</p>
        <div className="mt-6 flex gap-3">
          <button
            onClick={reset}
            className="rounded-lg bg-accent-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-accent-700"
          >
            Try again
          </button>
          <Link
            href="/"
            className="rounded-lg border border-zinc-200 px-3 py-2 text-sm font-medium text-zinc-700 hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-white/5"
          >
            Back to dashboard
          </Link>
        </div>
      </div>
    </main>
  );
}
