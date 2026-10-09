import { getTranslations } from 'next-intl/server';

import { LandingBenefits } from '@/app/(public)/welcome/_components/landing-benefits';
import { LandingCta } from '@/app/(public)/welcome/_components/landing-cta';
import { LandingFaq } from '@/app/(public)/welcome/_components/landing-faq';
import { LandingFeatures } from '@/app/(public)/welcome/_components/landing-features';
import { LandingHero } from '@/app/(public)/welcome/_components/landing-hero';
import { LandingSteps } from '@/app/(public)/welcome/_components/landing-steps';
import { LandingTrust } from '@/app/(public)/welcome/_components/landing-trust';
import { ROUTES } from '@/config/routes';
import { getAccessToken } from '@/lib/auth/session';
import { generatePublicPageMetadata } from '@/lib/utils/page';
import { siteOrigin } from '@/lib/utils/site-origin';

// The landing's canonical address is `/`, where the gate serves it; this path is only its route.
export const generateMetadata = () => generatePublicPageMetadata('landing', ROUTES.home);

export default async function LandingPage() {
  const t = await getTranslations('landing');
  const signedIn = (await getAccessToken()) !== undefined;
  const origin = await siteOrigin();
  const structuredData = {
    '@context': 'https://schema.org',
    '@graph': [
      { '@type': 'Organization', name: 'Coti', url: origin, logo: `${origin}/icon.png` },
      {
        '@type': 'SoftwareApplication',
        name: 'Coti',
        applicationCategory: 'BusinessApplication',
        operatingSystem: 'Web',
        inLanguage: 'es-AR',
        description: t('hero.description'),
        url: origin,
      },
    ],
  };

  return (
    <>
      <script
        type="application/ld+json"
        // Escaped so no value can close the script element early.
        dangerouslySetInnerHTML={{
          __html: JSON.stringify(structuredData).replace(/</g, '\\u003c'),
        }}
      />
      <LandingHero signedIn={signedIn} />
      <LandingBenefits />
      <LandingSteps />
      <LandingFeatures />
      <LandingTrust />
      <LandingFaq />
      {signedIn ? null : <LandingCta />}
    </>
  );
}
