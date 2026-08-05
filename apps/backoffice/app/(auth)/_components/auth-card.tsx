import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';

interface AuthCardProps {
  title: string;
  description?: string;
  children: React.ReactNode;
  /* A link out of the flow — back to login, request another link. */
  footer?: React.ReactNode;
  className?: string;
}

/*
 * The shell every auth screen shares, so the four flows can't drift in title size, spacing or where
 * the escape link sits. A flow's terminal state renders a StatusScreen in a bare Card instead — it
 * has no title row, because its icon and heading are the title.
 */
export function AuthCard({ title, description, children, footer, className }: AuthCardProps) {
  return (
    <Card className={cn('gap-y-6', className)}>
      <CardHeader className="items-center text-center">
        <CardTitle className="text-heading-4">{title}</CardTitle>
        {description ? (
          <p className="text-paragraph-sm text-foreground-muted">{description}</p>
        ) : null}
      </CardHeader>
      <CardContent>{children}</CardContent>
      {footer ? <CardFooter className="justify-center">{footer}</CardFooter> : null}
    </Card>
  );
}
