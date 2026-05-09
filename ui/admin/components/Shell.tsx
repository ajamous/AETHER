import Link from 'next/link';
import type { ReactNode } from 'react';

const NAV = [
  { href: '/', label: 'Dashboard' },
  { href: '/templates', label: 'Profile templates' },
  { href: '/certs', label: 'Certificates' },
  { href: '/smds', label: 'Discovery (SM-DS)' },
  { href: '/audit', label: 'Audit log' },
  { href: '/about', label: 'About' },
];

export function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen flex">
      <aside className="w-64 shrink-0 border-r border-zinc-200 dark:border-zinc-800 p-6">
        <div className="text-lg font-semibold tracking-tight">Aether</div>
        <div className="text-xs text-zinc-500 dark:text-zinc-400 mt-1 mb-6">
          Open Source RSP — admin
        </div>
        <nav className="flex flex-col gap-1 text-sm">
          {NAV.map((n) => (
            <Link
              key={n.href}
              href={n.href}
              className="px-3 py-1.5 rounded hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              {n.label}
            </Link>
          ))}
        </nav>
        <div className="mt-10 text-xs text-zinc-500 dark:text-zinc-400">
          <div>Status: Phase 0/1</div>
          <div className="mt-1">No auth in lab. Don&apos;t expose this UI.</div>
        </div>
      </aside>
      <main className="flex-1 p-10 max-w-5xl">{children}</main>
    </div>
  );
}
