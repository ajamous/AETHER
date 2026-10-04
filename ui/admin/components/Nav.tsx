'use client';

// Sidebar navigation (desktop) and the collapsible menu (mobile).
// Client-side only for usePathname (active link) and the menu toggle;
// everything else in the shell stays a server component.

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState, type ComponentType, type SVGProps } from 'react';
import {
  AuditIcon,
  CertIcon,
  CloseIcon,
  DashboardIcon,
  DeviceIcon,
  DiscoveryIcon,
  InfoIcon,
  LogoMark,
  MenuIcon,
  TemplateIcon,
} from '@/components/icons';

type Item = { href: string; label: string; icon: ComponentType<SVGProps<SVGSVGElement>> };

const SECTIONS: { heading?: string; items: Item[] }[] = [
  { items: [{ href: '/', label: 'Dashboard', icon: DashboardIcon }] },
  {
    heading: 'Provisioning',
    items: [
      { href: '/templates', label: 'Profile templates', icon: TemplateIcon },
      { href: '/smds', label: 'Discovery (SM-DS)', icon: DiscoveryIcon },
      { href: '/eim', label: 'IoT devices (eIM)', icon: DeviceIcon },
    ],
  },
  {
    heading: 'Trust',
    items: [
      { href: '/certs', label: 'Certificates', icon: CertIcon },
      { href: '/audit', label: 'Audit log', icon: AuditIcon },
    ],
  },
  { items: [{ href: '/about', label: 'About', icon: InfoIcon }] },
];

function isActive(pathname: string, href: string) {
  return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(`${href}/`);
}

export function Brand() {
  return (
    <Link href="/" className="flex items-center gap-2.5 rounded-md">
      <span className="grid size-8 place-items-center rounded-lg bg-accent-600 text-white shadow-sm">
        <LogoMark className="size-[18px]" />
      </span>
      <span className="leading-tight">
        <span className="block text-sm font-semibold tracking-tight text-zinc-900 dark:text-zinc-50">
          Aether
        </span>
        <span className="block text-[11px] text-zinc-500 dark:text-zinc-400">Operator console</span>
      </span>
    </Link>
  );
}

export function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname() || '/';
  return (
    <nav aria-label="Main" className="flex flex-col gap-5">
      {SECTIONS.map((section, i) => (
        <div key={section.heading ?? i}>
          {section.heading && (
            <div className="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
              {section.heading}
            </div>
          )}
          <ul className="flex flex-col gap-0.5">
            {section.items.map(({ href, label, icon: Icon }) => {
              const active = isActive(pathname, href);
              return (
                <li key={href}>
                  <Link
                    href={href}
                    onClick={onNavigate}
                    aria-current={active ? 'page' : undefined}
                    className={`flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors ${
                      active
                        ? 'bg-accent-50 font-medium text-accent-700 dark:bg-accent-500/10 dark:text-accent-300'
                        : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-white/5 dark:hover:text-zinc-100'
                    }`}
                  >
                    <Icon
                      className={`size-4 shrink-0 ${active ? 'text-accent-600 dark:text-accent-400' : 'text-zinc-400 dark:text-zinc-500'}`}
                    />
                    {label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}

// Below the lg breakpoint the sidebar is replaced by a top bar whose
// menu button reveals the same links.
export function MobileNav({ footer }: { footer?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const pathname = usePathname();

  // Close on route change (back/forward included).
  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  return (
    <header className="lg:hidden">
      <div className="flex h-14 items-center justify-between border-b border-zinc-200 bg-white/90 px-4 backdrop-blur dark:border-zinc-800 dark:bg-ink-900/90">
        <Brand />
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          aria-controls="mobile-nav"
          className="grid size-9 place-items-center rounded-md text-zinc-600 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-white/5"
        >
          {open ? (
            <CloseIcon className="size-5" title="Close menu" />
          ) : (
            <MenuIcon className="size-5" title="Open menu" />
          )}
        </button>
      </div>
      {open && (
        <div
          id="mobile-nav"
          className="border-b border-zinc-200 bg-white px-4 py-4 shadow-sm dark:border-zinc-800 dark:bg-ink-900"
        >
          <NavLinks onNavigate={() => setOpen(false)} />
          {footer && <div className="mt-5">{footer}</div>}
        </div>
      )}
    </header>
  );
}
