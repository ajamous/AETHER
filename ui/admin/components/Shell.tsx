import type { ReactNode } from 'react';
import { auth, oidcEnabled, signOut } from '@/auth';
import { AlertIcon } from '@/components/icons';
import { Brand, MobileNav, NavLinks } from '@/components/Nav';

export async function Shell({ children }: { children: ReactNode }) {
  const session = oidcEnabled ? await auth() : null;
  const user = session?.user;

  const account =
    oidcEnabled && user ? (
      <div className="flex items-center gap-3 rounded-lg border border-zinc-200 p-2.5 dark:border-zinc-800">
        <span
          className="grid size-8 shrink-0 place-items-center rounded-full bg-zinc-200 text-xs font-semibold uppercase text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200"
          aria-hidden
        >
          {(user.name || user.email || '?').slice(0, 1)}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-medium text-zinc-800 dark:text-zinc-100">
            {user.name || user.email || 'Signed in'}
          </div>
          {user.name && user.email && (
            <div className="truncate text-[11px] text-zinc-500 dark:text-zinc-400">
              {user.email}
            </div>
          )}
        </div>
        <form
          action={async () => {
            'use server';
            await signOut({ redirectTo: '/signin' });
          }}
        >
          <button
            type="submit"
            className="rounded px-1.5 py-1 text-[11px] font-medium text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 dark:text-zinc-400 dark:hover:bg-white/5 dark:hover:text-zinc-100"
          >
            Sign out
          </button>
        </form>
      </div>
    ) : (
      <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-xs dark:border-amber-500/30 dark:bg-amber-500/10">
        <div className="flex items-center gap-1.5 font-semibold text-amber-800 dark:text-amber-300">
          <AlertIcon className="size-3.5" />
          Auth disabled
        </div>
        <p className="mt-1 text-amber-800 dark:text-amber-200/90">
          Lab mode: no OIDC configured. Don&apos;t expose this UI on a network.
        </p>
      </div>
    );

  return (
    <div className="min-h-screen lg:flex">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-md focus:bg-white focus:px-3 focus:py-2 focus:text-sm focus:shadow dark:focus:bg-ink-800"
      >
        Skip to content
      </a>

      <aside className="hidden w-64 shrink-0 border-r border-zinc-200 bg-white lg:block dark:border-zinc-800 dark:bg-ink-900">
        <div className="sticky top-0 flex h-screen flex-col gap-8 overflow-y-auto px-4 py-5">
          <div className="px-1">
            <Brand />
          </div>
          <NavLinks />
          <div className="mt-auto">{account}</div>
        </div>
      </aside>

      <div className="min-w-0 flex-1">
        <MobileNav footer={account} />
        {!oidcEnabled && (
          <div
            role="region"
            aria-label="Lab mode warning"
            className="flex items-center justify-center gap-2 border-b border-amber-300 bg-amber-100 px-4 py-2 text-center text-xs font-medium text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/15 dark:text-amber-200"
          >
            <AlertIcon className="size-3.5 shrink-0" />
            <span>
              Lab mode — authentication is disabled. Configure OIDC before exposing this console.
            </span>
          </div>
        )}
        <main id="main" className="mx-auto w-full max-w-6xl px-4 py-8 sm:px-6 lg:px-10 lg:py-10">
          {children}
        </main>
      </div>
    </div>
  );
}
