import type { Metadata } from 'next';

import './globals.css';

export const metadata: Metadata = {
  title: 'Coti',
  description: 'Review and respond to your quote.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="es">
      <body>{children}</body>
    </html>
  );
}
