/*
 * The frame every settings page shares. The sections are listed in the context column from lg up and
 * in the menu sheet below it.
 */
export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-col min-w-0 px-6 py-10 gap-y-8 lg:px-10">{children}</div>;
}
