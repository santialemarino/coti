// Middleware bounces a signed-in caller off these routes before they render, so
// this only carries the shared frame.
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-6 py-10">
      <div className="flex flex-col w-full max-w-sm gap-y-6">{children}</div>
    </main>
  );
}
