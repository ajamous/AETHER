import type { Metadata } from 'next';
import { redirect } from 'next/navigation';
import { signIn, oidcEnabled } from '@/auth';
import { LogoMark } from '@/components/icons';

export const metadata: Metadata = { title: 'Sign in' };

// Sign-in entry. Renders only the OIDC button when configured;
// when auth is disabled (lab mode), it bounces back to the
// dashboard immediately so the URL still works as a no-op.

export default function SignInPage() {
  if (!oidcEnabled) {
    redirect('/');
  }

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center text-center">
          <span className="grid size-12 place-items-center rounded-xl bg-accent-600 text-white shadow-sm">
            <LogoMark className="size-6" />
          </span>
          <h1 className="mt-4 text-xl font-semibold tracking-tight">Sign in to Aether</h1>
          <p className="mt-1 text-sm text-zinc-500 dark:text-zinc-400">Operator console</p>
        </div>
        <div className="rounded-xl border border-zinc-200 bg-white p-6 shadow-xs dark:border-zinc-800 dark:bg-ink-900">
          <p className="mb-5 text-sm text-zinc-600 dark:text-zinc-300">
            Continue with your organisation&apos;s identity provider.
          </p>
          <form
            action={async () => {
              'use server';
              await signIn('oidc', { redirectTo: '/' });
            }}
          >
            <button
              type="submit"
              className="w-full rounded-lg bg-accent-600 px-3 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-accent-700"
            >
              Sign in with OIDC
            </button>
          </form>
        </div>
      </div>
    </main>
  );
}
