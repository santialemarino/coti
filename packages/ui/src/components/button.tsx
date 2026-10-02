import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

/*
 * Every variant carries all four interaction states: hover, active, focus-visible and disabled.
 * The focus ring replaces the browser outline and is tinted per variant, so a destructive button's
 * ring reads as destructive. Transitions enumerate their properties rather than using
 * `transition-all`, which would also animate a width change and make resizes look wobbly.
 *
 * A press is a colour step, never a scale. A control that shrinks under the pointer moves its own
 * edges away from it, so a press that started a few pixels inside the border releases outside the
 * shrunken box and the browser dispatches the click on the parent instead — the button lights up and
 * does nothing. Every variant therefore steps one rung deeper than its hover and stays put.
 */
const buttonVariants = cva(
  cn(
    'group/button inline-flex shrink-0 items-center justify-center whitespace-nowrap border border-transparent',
    'transition-[color,background-color,border-color,box-shadow,text-decoration-color] duration-200 ease-out-soft',
    'outline-none select-none',
    'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
    'disabled:pointer-events-none disabled:opacity-50',
    'aria-disabled:pointer-events-none aria-disabled:opacity-50',
    "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  ),
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground shadow-e1 hover:bg-primary-hover active:bg-primary-active',
        secondary:
          'bg-secondary text-secondary-foreground hover:bg-secondary-hover active:bg-surface-active aria-expanded:bg-secondary-hover',
        outline:
          'border-border bg-background text-foreground shadow-e1 hover:border-border-strong hover:bg-muted active:bg-surface-hover aria-expanded:border-border-strong aria-expanded:bg-muted',
        ghost:
          'text-foreground hover:bg-accent hover:text-accent-foreground active:bg-accent-strong aria-expanded:bg-accent aria-expanded:text-accent-foreground',
        destructive:
          'bg-danger text-white shadow-e1 hover:bg-danger-hover active:bg-danger-active focus-visible:border-danger focus-visible:ring-danger/40',
        link: 'text-primary underline decoration-transparent underline-offset-4 hover:decoration-primary focus-visible:border-transparent focus-visible:ring-0 focus-visible:animate-focus-bump-subtle',
      },
      size: {
        xs: 'h-6 px-2 gap-x-1 rounded-md text-paragraph-xs-medium',
        sm: 'h-8 px-3 gap-x-1.5 rounded-lg text-paragraph-sm-medium',
        default: 'h-9 px-4 gap-x-2 rounded-lg text-paragraph-sm-medium',
        lg: 'h-11 px-5 gap-x-2 rounded-lg text-paragraph-medium',
        'icon-xs': 'size-6 rounded-md',
        'icon-sm': 'size-8 rounded-lg',
        icon: 'size-9 rounded-lg',
        'icon-lg': 'size-11 rounded-lg',
      },
    },
    /*
     * An icon-only button has no label to shift colour, so keyboard focus gets the hover background
     * as well as the ring — otherwise the ring alone has to carry a state that hover signals twice.
     */
    compoundVariants: [
      {
        variant: 'ghost',
        size: ['icon-xs', 'icon-sm', 'icon', 'icon-lg'],
        className: 'focus-visible:bg-accent focus-visible:text-accent-foreground',
      },
      {
        variant: 'outline',
        size: ['icon-xs', 'icon-sm', 'icon', 'icon-lg'],
        className: 'focus-visible:bg-muted',
      },
    ],
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

function Button({
  className,
  variant = 'default',
  size = 'default',
  asChild = false,
  type,
  ...props
}: React.ComponentProps<'button'> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : 'button';

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      /* A bare <button> in a form submits it. The design system is form-heavy, so a real submit
         opts in rather than every other button having to opt out. */
      type={asChild ? type : (type ?? 'button')}
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  );
}

export { Button, buttonVariants };
