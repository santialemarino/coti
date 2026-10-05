// The frame every settings page shares; the sections they belong to are listed in the context column.
export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return <div className="flex min-w-0 flex-col px-6 py-10 lg:px-10">{children}</div>;
}
