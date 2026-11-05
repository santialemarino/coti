'use client';

import { Toaster as Sonner } from 'sonner';

// Sonner's own bottom offsets; `--toast-lift` raises them over the phone tab bar while it shows.
const OFFSET = 'calc(24px + var(--toast-lift, 0px))';
const MOBILE_OFFSET = 'calc(16px + var(--toast-lift, 0px))';

/*
 * Sonner styled with the design system's own tokens rather than its defaults, so a toast reads as
 * part of the app. Mounted once in the root layout; call `toast.success` / `toast.error` from
 * anywhere. Reach for a toast only for a transient confirmation of something the user just did — a
 * standing message about the content is a `Callout`, and a field's rejection is a `FormMessage`.
 */
export function Toaster() {
  return (
    <Sonner
      position="bottom-right"
      offset={{ bottom: OFFSET }}
      mobileOffset={{ bottom: MOBILE_OFFSET }}
      toastOptions={{
        classNames: {
          toast:
            'group !gap-x-2.5 !bg-popover !border-border !rounded-xl !shadow-e3 !text-paragraph-sm !text-foreground',
          description: '!text-paragraph-xs !text-foreground-muted',
          actionButton: '!bg-primary !text-primary-foreground !rounded-lg',
          cancelButton: '!bg-secondary !text-secondary-foreground !rounded-lg',
          success: '[&_[data-icon]]:!text-success',
          error: '[&_[data-icon]]:!text-danger',
          warning: '[&_[data-icon]]:!text-warning',
          info: '[&_[data-icon]]:!text-primary',
        },
      }}
    />
  );
}
