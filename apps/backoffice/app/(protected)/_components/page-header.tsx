interface PageHeaderProps {
  title: string;
  description?: string;
  // Page-level actions, aligned to the title's end on a wide screen.
  actions?: React.ReactNode;
}

// A page's one heading. Cards below it take their own titles, so nothing repeats the page's name.
export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div className="flex min-w-0 flex-col gap-y-1">
        <h1 className="text-heading-2">{title}</h1>
        {description ? <p className="text-paragraph text-foreground-muted">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-x-3">{actions}</div> : null}
    </div>
  );
}
