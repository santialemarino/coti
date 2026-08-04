import type { MetadataRoute } from 'next';

/*
 * Web app manifest. Next serves it at /manifest.webmanifest and links it automatically.
 * The colours match the app surface rather than the brand blue, for the reason in layout.tsx.
 * `maskable` is a separate, more padded icon because Android crops to its own shape.
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: '/',
    name: 'Coti',
    short_name: 'Coti',
    description: 'Review and respond to your quote.',
    start_url: '/',
    display: 'standalone',
    lang: 'es-AR',
    dir: 'ltr',
    background_color: '#F2F7FB',
    theme_color: '#F2F7FB',
    icons: [
      { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
      {
        src: '/icons/icon-maskable-512.png',
        sizes: '512x512',
        type: 'image/png',
        purpose: 'maskable',
      },
    ],
  };
}
