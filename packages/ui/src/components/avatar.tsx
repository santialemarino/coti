'use client';

import * as React from 'react';
import * as AvatarPrimitive from '@radix-ui/react-avatar';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

const avatarVariants = cva('relative flex shrink-0 overflow-hidden rounded-full select-none', {
  variants: {
    size: {
      sm: 'size-7 text-paragraph-xs-semibold',
      default: 'size-9 text-paragraph-sm-semibold',
      lg: 'size-12 text-paragraph-semibold',
    },
  },
  defaultVariants: { size: 'default' },
});

function Avatar({
  className,
  size = 'default',
  ...props
}: React.ComponentProps<typeof AvatarPrimitive.Root> & VariantProps<typeof avatarVariants>) {
  return (
    <AvatarPrimitive.Root
      data-slot="avatar"
      className={cn(avatarVariants({ size }), className)}
      {...props}
    />
  );
}

function AvatarImage({ className, ...props }: React.ComponentProps<typeof AvatarPrimitive.Image>) {
  return (
    <AvatarPrimitive.Image
      data-slot="avatar-image"
      className={cn('aspect-square size-full object-cover', className)}
      {...props}
    />
  );
}

/*
 * The fallback is the common case here — the product has no avatar upload, so this renders initials
 * on a brand tint for every user.
 */
function AvatarFallback({
  className,
  ...props
}: React.ComponentProps<typeof AvatarPrimitive.Fallback>) {
  return (
    <AvatarPrimitive.Fallback
      data-slot="avatar-fallback"
      className={cn(
        'grid size-full place-items-center bg-accent-strong text-accent-foreground uppercase',
        className,
      )}
      {...props}
    />
  );
}

export { Avatar, AvatarFallback, AvatarImage, avatarVariants };
