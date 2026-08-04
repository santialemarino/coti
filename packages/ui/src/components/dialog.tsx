'use client';

import * as React from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { XIcon } from 'lucide-react';

import { cn } from '../lib/utils';

function Dialog({ ...props }: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />;
}

function DialogTrigger({ ...props }: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />;
}

function DialogPortal({ ...props }: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />;
}

function DialogClose({ ...props }: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />;
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        'fixed inset-0 z-50 bg-backdrop',
        'data-[state=open]:animate-in data-[state=open]:fade-in-0',
        'data-[state=closed]:animate-out data-[state=closed]:fade-out-0',
        'duration-200 ease-out-soft',
        className,
      )}
      {...props}
    />
  );
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  closeOnClickOutside = true,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean;
  /* Set false for a dialog with unsaved input, so a stray click outside can't discard it. */
  closeOnClickOutside?: boolean;
}) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        className={cn(
          'fixed top-1/2 left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] sm:max-w-xl -translate-x-1/2 -translate-y-1/2',
          /* A form taller than the viewport would otherwise push its own footer off-screen with no
             way to reach it. dvh, not vh, so mobile browser chrome is accounted for. */
          'max-h-[90dvh] overflow-y-auto',
          'gap-y-4 px-4 py-6 sm:px-6 bg-background border border-border rounded-2xl shadow-e4',
          'focus:outline-none',
          'data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
          'data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95',
          'duration-200 ease-out-soft',
          className,
        )}
        onPointerDownOutside={(e) => {
          if (!closeOnClickOutside) e.preventDefault();
          props.onPointerDownOutside?.(e);
        }}
        onInteractOutside={(e) => {
          if (!closeOnClickOutside) e.preventDefault();
          props.onInteractOutside?.(e);
        }}
        {...props}
      >
        {children}
        {showCloseButton ? (
          /* Icon-only trigger: no rectangular ring. Focus is a colour shift plus the icon bump, so
             the affordance survives when reduced motion removes the bump. */
          <DialogPrimitive.Close
            data-slot="dialog-close"
            className={cn(
              'group/dialog-close absolute top-4 right-4 flex p-1 rounded-md outline-none',
              'transition-colors duration-200 ease-out-soft',
              'text-foreground-subtle hover:text-foreground focus-visible:text-foreground',
              'disabled:pointer-events-none',
            )}
          >
            <XIcon
              aria-hidden="true"
              className="size-4 group-focus-visible/dialog-close:animate-focus-bump"
            />
            <span className="sr-only">Cerrar</span>
          </DialogPrimitive.Close>
        ) : null}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="dialog-header"
      className={cn('flex flex-col gap-y-1.5 pr-8', className)}
      {...props}
    />
  );
}

function DialogFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn('flex flex-col-reverse sm:flex-row sm:justify-end gap-2', className)}
      {...props}
    />
  );
}

function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn('text-heading-5 text-foreground', className)}
      {...props}
    />
  );
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn('text-paragraph-sm text-foreground-muted', className)}
      {...props}
    />
  );
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
};
