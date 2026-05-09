import type { ReactNode } from 'react';

export function Card({
  title,
  children,
  footer,
}: {
  title?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <section className="border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden mb-6">
      {title && (
        <header className="px-5 py-3 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/40">
          <h2 className="text-sm font-medium text-zinc-700 dark:text-zinc-200">{title}</h2>
        </header>
      )}
      <div className="p-5">{children}</div>
      {footer && (
        <footer className="px-5 py-3 border-t border-zinc-200 dark:border-zinc-800 text-xs text-zinc-500 dark:text-zinc-400">
          {footer}
        </footer>
      )}
    </section>
  );
}

export function StatusDot({ ok }: { ok: boolean | null | undefined }) {
  const color = ok === true ? 'bg-emerald-500' : ok === false ? 'bg-red-500' : 'bg-zinc-400';
  return <span className={`inline-block w-2 h-2 rounded-full ${color} mr-2 align-middle`} />;
}

export function Empty({ message }: { message: string }) {
  return <div className="text-sm text-zinc-500 italic">{message}</div>;
}
