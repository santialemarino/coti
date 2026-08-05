import { clsx, type ClassValue } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

/*
 * tailwind-merge can't know the project's type scale, so it reads `text-heading-3` and
 * `text-paragraph-sm` as unrelated to each other and to `text-sm` — two of them in one className
 * would both survive and the loser would win on source order. Registering them as font-size makes
 * the last one win, which is what a caller overriding a component's default size expects. The same
 * applies to the elevation scale. Every token in either scale must be listed here; see
 * packages/ui/src/styles/theme.css.
 */
const customTwMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [
        'text-heading-1',
        'text-heading-2',
        'text-heading-3',
        'text-heading-4',
        'text-heading-5',
        'text-heading-6',
        'text-paragraph',
        'text-paragraph-medium',
        'text-paragraph-semibold',
        'text-paragraph-sm',
        'text-paragraph-sm-medium',
        'text-paragraph-sm-semibold',
        'text-paragraph-xs',
        'text-paragraph-xs-medium',
        'text-paragraph-xs-semibold',
        'text-paragraph-mini',
        'text-paragraph-mini-medium',
        'text-paragraph-mini-semibold',
      ],
      shadow: ['shadow-e1', 'shadow-e2', 'shadow-e3', 'shadow-e4'],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return customTwMerge(clsx(inputs));
}
