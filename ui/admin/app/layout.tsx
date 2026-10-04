import './globals.css';
import type { Metadata, Viewport } from 'next';

export const metadata: Metadata = {
  title: { default: 'Aether Admin', template: '%s · Aether Admin' },
  description: 'Aether — Open Source Remote SIM Provisioning. Operator console.',
};

// Render every route per request. Auth state (OIDC on/off, the
// session) comes from runtime env and cookies; a page prerendered at
// build time would freeze the build environment's answer. That froze
// /signin into "OIDC off → redirect('/')" while the middleware, reading
// the real env, sent users to /signin: an endless redirect loop.
export const dynamic = 'force-dynamic';

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: '#fafafa' },
    { media: '(prefers-color-scheme: dark)', color: '#0b0d11' },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
