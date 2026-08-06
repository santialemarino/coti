'use client';

import { CheckIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Progress } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { PASSWORD_CHECKS, PASSWORD_MIN_LENGTH, type PasswordCheck } from '@/lib/constants/password';

type Strength = 'empty' | 'weak' | 'moderate' | 'strong';

/* Bucketed rather than proportional: the bar reports how close the password is, not how many rules
   it happens to satisfy, and the three named steps are what the label names. */
const STRENGTH: Record<
  Strength,
  { fill: number; tone: 'neutral' | 'danger' | 'warning' | 'success'; text: string }
> = {
  empty: { fill: 0, tone: 'neutral', text: 'text-foreground-subtle' },
  weak: { fill: 25, tone: 'danger', text: 'text-danger-foreground' },
  moderate: { fill: 75, tone: 'warning', text: 'text-warning-foreground' },
  strong: { fill: 100, tone: 'success', text: 'text-success-foreground' },
};

function strengthOf(met: number): Strength {
  if (met === PASSWORD_CHECKS.length) return 'strong';
  if (met >= 3) return 'moderate';
  if (met >= 1) return 'weak';
  return 'empty';
}

interface PasswordMeterProps {
  checks: Record<PasswordCheck, boolean>;
  /* After a rejected submit, what is still missing is an error rather than a hint. */
  invalid?: boolean;
}

/*
 * What the password still needs, and how far along it is. It replaces a "mínimo N caracteres" hint:
 * the rules are the API's own, so listing them is the only way a caller can satisfy them on the
 * first try instead of by resubmitting.
 */
export function PasswordMeter({ checks, invalid = false }: PasswordMeterProps) {
  const t = useTranslations('common.passwordMeter');
  const strength = strengthOf(Object.values(checks).filter(Boolean).length);
  const style = STRENGTH[strength];

  return (
    <div className="flex flex-col gap-y-3">
      <div className="flex items-center gap-x-2">
        <span className="shrink-0 text-paragraph-xs-medium text-foreground-muted">
          {t('title')}
        </span>
        <Progress value={style.fill} tone={style.tone} size="sm" label={t('title')} />
        {strength === 'empty' ? null : (
          <span className={cn('shrink-0 text-paragraph-xs-medium', style.text)}>
            {t(`strength.${strength}`)}
          </span>
        )}
      </div>

      <div className="flex flex-col gap-y-1.5">
        <span className="text-paragraph-xs-medium text-foreground-muted">
          {t('requirements.title')}
        </span>
        <ul className="flex flex-col gap-y-1">
          {PASSWORD_CHECKS.map((check) => (
            <Requirement
              key={check}
              met={checks[check]}
              missing={invalid && !checks[check]}
              label={t(`requirements.${check}`, { count: PASSWORD_MIN_LENGTH })}
            />
          ))}
        </ul>
      </div>
    </div>
  );
}

/*
 * The two markers are stacked in one grid cell and crossfaded, so a requirement turning green never
 * reflows the list — the same shape the password reveal toggle uses for its own pair of icons.
 */
function Requirement({ met, missing, label }: { met: boolean; missing: boolean; label: string }) {
  return (
    <li className="flex items-center gap-x-2">
      <span aria-hidden="true" className="grid size-3.5 shrink-0 place-items-center">
        <CheckIcon
          className={cn(
            'col-start-1 row-start-1 size-3.5 text-success transition-opacity duration-200 ease-out-soft',
            met ? 'opacity-100' : 'opacity-0',
          )}
        />
        <span
          className={cn(
            'col-start-1 row-start-1 size-1.5 rounded-full transition-[opacity,background-color] duration-200 ease-out-soft',
            missing ? 'bg-danger' : 'bg-foreground-subtle',
            met ? 'opacity-0' : 'opacity-100',
          )}
        />
      </span>
      <span
        className={cn(
          'text-paragraph-xs transition-colors duration-200 ease-out-soft',
          missing ? 'text-danger-foreground' : 'text-foreground-muted',
        )}
      >
        {label}
      </span>
    </li>
  );
}
