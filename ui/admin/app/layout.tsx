import './globals.css';
import type { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Aether Admin',
  description: 'Aether — Open Source Remote SIM Provisioning. Operator console.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="bg-white text-zinc-900 dark:bg-[#0b0d11] dark:text-zinc-100 antialiased">
        {children}
      </body>
    </html>
  );
}
