import Link from 'next/link';
import { KeyRoundIcon, LogOutIcon, SettingsIcon } from 'lucide-react';
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
import { signOut } from '@/app/(protected)/actions';
import { Brand } from '@/components/brand';
import { ROUTES } from '@/config/routes';
import { getBranches } from '@/lib/api/branches';
import { getActiveBranchId } from '@/lib/auth/branch';
import type { SessionUser } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

interface AppHeaderProps {
  session: SessionUser;
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

export async function AppHeader({ session }: AppHeaderProps) {
  const t = await getTranslations('common');
  const branches = await getBranches();
  const activeBranchId = await getActiveBranchId();

  return (
    <header className="sticky top-0 z-40 flex h-16 shrink-0 items-center justify-between px-6 bg-background/85 border-b border-border backdrop-blur">
      <Link
        href={ROUTES.home}
        aria-label={t('appName')}
        className="flex items-center rounded-md outline-none focus-visible:animate-focus-bump-subtle"
      >
        <Brand variant="wordmark" size="md" />
      </Link>

      <div className="flex items-center gap-x-2">
        {/* One reachable branch is the caller's whole reach, so there is nothing to switch. */}
        {branches.length > 1 ? (
          <BranchSwitcher branches={branches} activeBranchId={activeBranchId ?? null} />
        ) : null}

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            {/* Two stacked lines beside a 28px avatar need more room than any fixed size gives, so
                the trigger sizes to its content with its own padding. */}
            <Button variant="ghost" size="sm" className="gap-x-2 h-auto py-1.5 pl-2 pr-3">
              <Avatar size="sm">
                <AvatarFallback>{initials(session.name)}</AvatarFallback>
              </Avatar>
              <span className="hidden sm:flex flex-col items-start">
                <span className="text-paragraph-sm-medium text-foreground">{session.name}</span>
                <span className="text-paragraph-mini text-foreground-muted">
                  {t(`roles.${session.role}`)}
                </span>
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="min-w-52">
            <DropdownMenuLabel>{t(`roles.${session.role}`)}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {session.role === ADMIN_ROLE ? (
              <DropdownMenuItem asChild>
                <Link href={ROUTES.accountSettings}>
                  <SettingsIcon aria-hidden="true" />
                  {t('nav.settings')}
                </Link>
              </DropdownMenuItem>
            ) : null}
            <DropdownMenuItem asChild>
              <Link href={ROUTES.changePassword}>
                <KeyRoundIcon aria-hidden="true" />
                {t('nav.changePassword')}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
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
