// Presentational building blocks for the console. Server-safe: no
// hooks, no client state.

import Link from 'next/link';
import type { ComponentType, ReactNode, SVGProps } from 'react';
import { absoluteTime, relativeTime, type Tone } from '@/lib/format';
import { ArrowRightIcon, InboxIcon, PlugOffIcon } from '@/components/icons';

export function PageHeader({
  title,
  description,
  meta,
}: {
  title: string;
  description?: ReactNode;
  meta?: ReactNode;
}) {
  return (
    <header className="mb-8 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-50">
          {title}
        </h1>
        {description && (
          <p className="mt-1.5 max-w-2xl text-sm text-zinc-500 dark:text-zinc-400">{description}</p>
        )}
      </div>
      {meta && <div className="flex flex-wrap items-center gap-2">{meta}</div>}
    </header>
  );
}

export function Card({
  title,
  description,
  action,
  children,
  footer,
  flush = false,
  className = '',
}: {
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  // Drop body padding so a table can run edge to edge.
  flush?: boolean;
  className?: string;
}) {
  return (
    <section
      className={`overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-xs dark:border-zinc-800 dark:bg-ink-900 ${className}`}
    >
      {(title || action) && (
        <header className="flex items-start justify-between gap-4 border-b border-zinc-200 px-5 py-3.5 dark:border-zinc-800">
          <div className="min-w-0">
            {title && (
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">{title}</h2>
            )}
            {description && (
              <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">{description}</p>
            )}
          </div>
          {action && <div className="shrink-0">{action}</div>}
        </header>
      )}
      <div className={flush ? '' : 'p-5'}>{children}</div>
      {footer && (
        <footer className="border-t border-zinc-200 bg-zinc-50/60 px-5 py-2.5 text-xs text-zinc-500 dark:border-zinc-800 dark:bg-white/[0.02] dark:text-zinc-400">
          {footer}
        </footer>
      )}
    </section>
  );
}

const BADGE: Record<Tone, string> = {
  ok: 'bg-emerald-50 text-emerald-700 ring-emerald-600/20 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-400/25',
  warn: 'bg-amber-50 text-amber-800 ring-amber-600/25 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-400/25',
  danger:
    'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-500/10 dark:text-red-300 dark:ring-red-400/25',
  info: 'bg-accent-50 text-accent-700 ring-accent-600/20 dark:bg-accent-500/10 dark:text-accent-300 dark:ring-accent-400/25',
  neutral:
    'bg-zinc-100 text-zinc-600 ring-zinc-500/20 dark:bg-zinc-500/10 dark:text-zinc-300 dark:ring-zinc-400/20',
};

const DOT: Record<Tone, string> = {
  ok: 'bg-emerald-500',
  warn: 'bg-amber-500',
  danger: 'bg-red-500',
  info: 'bg-accent-500',
  neutral: 'bg-zinc-400',
};

// Status is always carried by the label text; the colour and dot only
// reinforce it (WCAG 1.4.1, use of colour).
export function Badge({
  tone = 'neutral',
  dot = false,
  children,
}: {
  tone?: Tone;
  dot?: boolean;
  children: ReactNode;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${BADGE[tone]}`}
    >
      {dot && <span className={`size-1.5 rounded-full ${DOT[tone]}`} aria-hidden />}
      {children}
    </span>
  );
}

export function StatTile({
  label,
  value,
  hint,
  tone,
  href,
  icon: Icon,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: Tone;
  href?: string;
  icon?: ComponentType<SVGProps<SVGSVGElement>>;
}) {
  const body = (
    <>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-zinc-500 dark:text-zinc-400">{label}</span>
        {Icon && <Icon className="size-4 text-zinc-400 dark:text-zinc-500" />}
      </div>
      <div className="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-zinc-900 dark:text-zinc-50">
        {value}
      </div>
      {hint && (
        <div className="mt-1.5 flex min-h-5 items-center text-xs text-zinc-500 dark:text-zinc-400">
          {tone ? (
            <Badge tone={tone} dot>
              {hint}
            </Badge>
          ) : (
            hint
          )}
        </div>
      )}
    </>
  );
  const cls =
    'block rounded-xl border border-zinc-200 bg-white p-4 shadow-xs dark:border-zinc-800 dark:bg-ink-900';
  return href ? (
    <Link
      href={href}
      className={`${cls} transition-colors hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-zinc-700 dark:hover:bg-ink-800`}
    >
      {body}
    </Link>
  ) : (
    <div className={cls}>{body}</div>
  );
}

// Tables scroll horizontally inside their card instead of pushing the
// page wider than the viewport.
export function Table({ children, label }: { children: ReactNode; label: string }) {
  return (
    // Focusable so keyboard users can scroll it (axe: scrollable-region-focusable).
    <div className="overflow-x-auto" role="region" aria-label={label} tabIndex={0}>
      <table className="w-full min-w-max text-left text-sm">{children}</table>
    </div>
  );
}

export function THead({ children }: { children: ReactNode }) {
  return (
    <thead className="border-b border-zinc-200 bg-zinc-50/80 text-xs text-zinc-500 dark:border-zinc-800 dark:bg-white/[0.02] dark:text-zinc-400">
      <tr>{children}</tr>
    </thead>
  );
}

export function Th({ children, className = '' }: { children?: ReactNode; className?: string }) {
  return (
    <th scope="col" className={`px-5 py-2.5 font-medium ${className}`}>
      {children}
    </th>
  );
}

export function TBody({ children }: { children: ReactNode }) {
  return <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800/70">{children}</tbody>;
}

export function Tr({ children }: { children: ReactNode }) {
  return <tr className="hover:bg-zinc-50/70 dark:hover:bg-white/[0.02]">{children}</tr>;
}

export function Td({ children, className = '' }: { children?: ReactNode; className?: string }) {
  return <td className={`px-5 py-3 align-top ${className}`}>{children}</td>;
}

export function Mono({
  children,
  muted = false,
  className = '',
}: {
  children: ReactNode;
  muted?: boolean;
  className?: string;
}) {
  const color = muted ? 'text-zinc-500 dark:text-zinc-400' : 'text-zinc-700 dark:text-zinc-300';
  return <span className={`font-mono text-[0.8125rem] ${color} ${className}`}>{children}</span>;
}

export function Time({ iso, now }: { iso: string; now?: number }) {
  return (
    <time dateTime={iso} title={absoluteTime(iso)} className="whitespace-nowrap">
      {relativeTime(iso, now)}
    </time>
  );
}

// No data yet. `hint` says how to create some.
export function Empty({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div className="flex flex-col items-center px-6 py-10 text-center">
      <InboxIcon className="size-8 text-zinc-300 dark:text-zinc-600" />
      <p className="mt-3 text-sm font-medium text-zinc-700 dark:text-zinc-200">{title}</p>
      {hint && <p className="mt-1 max-w-md text-xs text-zinc-500 dark:text-zinc-400">{hint}</p>}
    </div>
  );
}

// The backend didn't answer. Distinct from Empty: this is a fault.
export function Unreachable({ service, hint }: { service: string; hint?: ReactNode }) {
  return (
    <div className="flex flex-col items-center px-6 py-10 text-center" role="status">
      <PlugOffIcon className="size-8 text-red-400 dark:text-red-400/70" />
      <p className="mt-3 text-sm font-medium text-zinc-700 dark:text-zinc-200">
        Can&apos;t reach {service}
      </p>
      <p className="mt-1 max-w-md text-xs text-zinc-500 dark:text-zinc-400">
        {hint ??
          'Check that the service is running and that the console’s backend URLs point at it.'}
      </p>
    </div>
  );
}

// Honest-status panel: what a page doesn't do yet.
export function NotYet({ items }: { items: ReactNode[] }) {
  return (
    <div className="rounded-xl border border-dashed border-zinc-300 px-5 py-4 dark:border-zinc-700">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
        Not in this console yet
      </h2>
      <ul className="mt-2 space-y-1 text-sm text-zinc-600 dark:text-zinc-300">
        {items.map((item, i) => (
          <li key={i} className="flex gap-2">
            <span className="mt-2 size-1 shrink-0 rounded-full bg-zinc-400" aria-hidden />
            <span>{item}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function MoreLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-xs font-medium text-accent-600 hover:text-accent-700 dark:text-accent-400 dark:hover:text-accent-300"
    >
      {children}
      <ArrowRightIcon className="size-3.5" />
    </Link>
  );
}
