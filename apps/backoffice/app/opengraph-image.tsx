import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { ImageResponse } from 'next/og';

import messages from '@/translations/es.json';

export const alt = messages.meta.socialImageAlt;
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

/*
 * The card a shared link unfurls into. It renders outside the stylesheet, so its colours are the
 * literals behind the tokens: the brand ramp's 600 and 400 steps, the wordmark ink and the page wash.
 */
export default async function OpengraphImage() {
  const lockup = await readFile(join(process.cwd(), 'public/brand/lockup.png'));
  const lockupSrc = `data:image/png;base64,${lockup.toString('base64')}`;

  return new ImageResponse(
    <div
      style={{
        display: 'flex',
        width: '100%',
        height: '100%',
        alignItems: 'center',
        padding: 72,
        gap: 64,
        background: 'linear-gradient(135deg, #2F6CB3 0%, #35BCEE 100%)',
      }}
    >
      <div
        style={{
          display: 'flex',
          width: 300,
          height: 360,
          alignItems: 'center',
          justifyContent: 'center',
          background: '#F2F7FB',
          borderRadius: 32,
        }}
      >
        <img src={lockupSrc} width={198} height={240} alt="" />
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', flex: 1, gap: 24, color: '#FFFFFF' }}>
        <div style={{ fontSize: 30, letterSpacing: 2 }}>{messages.landing.hero.slogan}</div>
        <div style={{ fontSize: 60, fontWeight: 700, lineHeight: 1.1 }}>
          {messages.landing.hero.title}
        </div>
      </div>
    </div>,
    size,
  );
}
