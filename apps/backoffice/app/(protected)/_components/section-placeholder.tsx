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
    <Card className="gap-y-0 overflow-hidden py-0">
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
