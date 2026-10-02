import Link from 'next/link';
import type { LucideIcon } from 'lucide-react';
import { ArrowLeftIcon } from 'lucide-react';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';

interface SectionPlaceholderProps {
  icon: LucideIcon;
  title: string;
  description: string;
  backLabel: string;
}

/*
 * The landing screen for a top-level section whose real page does not exist yet. A placeholder is a
 * plain StatusScreen plus a way back, so every nav entry is a real destination; the day the section
 * ships, its page replaces this component instead of inheriting it.
 */
export function SectionPlaceholder({
  icon,
  title,
  description,
  backLabel,
}: SectionPlaceholderProps) {
  return (
    /* py-14, not the card's default py-6: the screen is the whole section, not a block inside one. */
    <Card className="gap-y-0 py-14">
      <StatusScreen icon={icon} tone="info" title={title} description={description}>
        <Button asChild variant="outline">
          <Link href={ROUTES.rfqs}>
            <ArrowLeftIcon aria-hidden="true" />
            {backLabel}
          </Link>
        </Button>
      </StatusScreen>
    </Card>
  );
}
