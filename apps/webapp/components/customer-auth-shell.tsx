import Image from 'next/image';
import { Building2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';

interface CustomerAuthShellProps {
  supplierName: string;
  logoUrl?: string;
  children: React.ReactNode;
}

/* The portal's sign-in frame: the supplier's mark above one narrow column, for every account step. */
export function CustomerAuthShell({ supplierName, logoUrl, children }: CustomerAuthShellProps) {
  const t = useTranslations('customerAuth');
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center px-4 py-10 gap-y-8 sm:px-6 sm:py-16">
      <div className="flex w-full max-w-md flex-col gap-y-8">
        <header className="flex items-center justify-center gap-x-3">
          {logoUrl ? (
            <Image
              src={logoUrl}
              alt=""
              width={40}
              height={40}
              unoptimized
              className="size-10 rounded-lg object-contain"
            />
          ) : (
            <span className="flex size-10 items-center justify-center bg-accent rounded-lg text-primary">
              <Building2Icon className="size-5" aria-hidden="true" />
            </span>
          )}
          <div className="flex min-w-0 flex-col gap-y-1">
            <p className="break-words text-heading-5">{supplierName}</p>
            <p className="text-paragraph-xs text-foreground-muted">{t('portalLabel')}</p>
          </div>
        </header>
        {children}
      </div>
    </main>
  );
}
