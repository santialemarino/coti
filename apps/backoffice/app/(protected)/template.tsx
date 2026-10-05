/*
 * Remounted on every navigation, so each screen fades in rather than cutting in while the queue
 * column slides beside it. Under reduced motion the shared gate drops the animation.
 */
export default function ProtectedTemplate({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-1 flex-col animate-in fade-in-0 duration-300 ease-out-soft">
      {children}
    </div>
  );
}
