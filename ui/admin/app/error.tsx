'use client';

// Minimal error fallback. Deliberately doesn't import Shell — Shell
// is an async server component with server-action sign-out forms,
// which can't render from inside a client component (which the
// error boundary must be).

export default function GlobalError({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div className="min-h-screen flex items-center justify-center px-4">
      <div className="w-full max-w-lg border border-zinc-200 dark:border-zinc-800 rounded-lg p-8">
        <h1 className="text-xl font-semibold mb-4">Something broke</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-300 mb-4">{error.message}</p>
        <button
          onClick={reset}
          className="px-3 py-1.5 rounded bg-accent-600 text-white text-sm hover:bg-accent-700"
        >
          Try again
        </button>
      </div>
    </div>
  );
}
