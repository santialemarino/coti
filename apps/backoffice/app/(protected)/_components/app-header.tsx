import Link from 'next/link';
import { LogOutIcon, MailIcon, SettingsIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
  Avatar,
  AvatarFallback,
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@repo/ui/components';
import { BranchSwitcher } from '@/app/(protected)/_components/branch-switcher';
import { ContextSheet } from '@/app/(protected)/_components/context-sheet';
import { PrimaryNav } from '@/app/(protected)/_components/primary-nav';
import type { SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { signOut } from '@/app/(protected)/actions';
import { AttentionDot } from '@/components/attention-dot';
import { Brand } from '@/components/brand';
import { ROUTES } from '@/config/routes';
import { getBranches } from '@/lib/api/branches';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import type { SessionUser } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { anyBranchMissesEmail } from '@/lib/utils/setup-issues';

// Clears the 64px header's bottom border: the trigger is ~51px tall, so it ends ~6px above it.
const PROFILE_MENU_OFFSET = 14;

interface AppHeaderProps {
  session: SessionUser;
  settingsNav: SettingsNavItem[];
}

/* First letters of the first two words, which is what a two-slot avatar can show. */
function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0])
    .join('')
    .toUpperCase();
}

export async function AppHeader({ session, settingsNav }: AppHeaderProps) {
  const t = await getTranslations('common');
  const branches = await getBranches();
  const activeBranchId = await getEffectiveBranchId(branches);
  const isAdmin = session.role === ADMIN_ROLE;
  // Something only an admin can fix, which Configuración leads to.
  const needsSetup = isAdmin && anyBranchMissesEmail(branches);
  // Reaching the shell unconfirmed means it is not required; it is still worth doing once mail can
  // actually bring the link.
  const canConfirmEmail = !session.emailVerified && session.mailDelivery;

  return (
    // Below lg the sections fold into the menu sheet and the profile keeps only its avatar, so the bar
    // fits a phone without scrolling sideways.
    <header className="sticky top-0 z-40 flex h-16 shrink-0 items-center px-4 gap-x-3 bg-background/85 border-b border-border backdrop-blur lg:px-6 lg:gap-x-8">
      <ContextSheet settingsNav={settingsNav} className="lg:hidden" />

      <Link
        href={ROUTES.home}
        aria-label={t('appName')}
        className="flex shrink-0 items-center rounded-md outline-none focus-visible:animate-focus-bump-subtle"
      >
        <Brand variant="wordmark" size="md" />
      </Link>

      <PrimaryNav className="max-lg:hidden" />

      <div className="ml-auto flex items-center gap-x-3">
        {/* Every reachable branch stays visible as working context. One branch is shown rather than
            offered; with more, admins keep the account-wide option alongside their branches. */}
        {branches.length > 0 ? (
          <BranchSwitcher
            branches={branches}
            activeBranchId={activeBranchId ?? null}
            isAdmin={isAdmin}
          />
        ) : null}

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            {/* Two stacked lines beside a 28px avatar need more room than any fixed size gives, so
                the trigger sizes to its content with its own padding. */}
            <Button variant="ghost" size="sm" className="gap-x-2 h-auto py-1.5 px-2 lg:pr-3">
              <span className="relative flex shrink-0">
                <Avatar size="sm">
                  <AvatarFallback>{initials(session.name)}</AvatarFallback>
                </Avatar>
                {needsSetup || canConfirmEmail ? (
                  <AttentionDot
                    label={t('nav.pending')}
                    className="absolute -top-0.5 -right-0.5 ring-2 ring-background"
                  />
                ) : null}
              </span>
              <span className="hidden lg:flex flex-col items-start">
                <span className="text-paragraph-sm-medium text-foreground">{session.name}</span>
                <span className="text-paragraph-mini text-foreground-muted">
                  {t(`roles.${session.role}`)}
                </span>
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent sideOffset={PROFILE_MENU_OFFSET} className="min-w-52">
            <DropdownMenuLabel>{t(`roles.${session.role}`)}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {canConfirmEmail ? (
              <DropdownMenuItem asChild>
                <Link href={ROUTES.verifyEmail}>
                  <MailIcon aria-hidden="true" />
                  {t('nav.confirmEmail')}
                  <AttentionDot label={t('nav.pending')} className="ml-auto" />
                </Link>
              </DropdownMenuItem>
            ) : null}
            {isAdmin ? (
              <DropdownMenuItem asChild>
                {/* With something to fix, the entry lands where it is fixed. */}
                <Link href={needsSetup ? ROUTES.branchSettings : ROUTES.accountSettings}>
                  <SettingsIcon aria-hidden="true" />
                  {t('nav.settings')}
                  {needsSetup ? (
                    <AttentionDot label={t('setup.attention')} className="ml-auto" />
                  ) : null}
                </Link>
              </DropdownMenuItem>
            ) : null}
            {isAdmin || canConfirmEmail ? <DropdownMenuSeparator /> : null}
            {/*
              Signing out is a POST, so it stays a form action rather than a link — and the menu item
              renders as the submit button so it keeps the menu's highlight and keyboard behaviour.
            */}
            <form action={signOut}>
              <DropdownMenuItem asChild tone="danger">
                <button type="submit" className="w-full">
                  <LogOutIcon aria-hidden="true" />
                  {t('nav.signOut')}
                </button>
              </DropdownMenuItem>
            </form>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
