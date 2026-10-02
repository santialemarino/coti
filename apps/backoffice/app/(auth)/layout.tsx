import { BrandedScreen } from '@/components/branded-screen';

// Only the shared frame. Most of these routes are signed-out-only and the gate bounces a
// signed-in caller before they render — verify-email and reset-password are the exceptions:
// signup lands on the first holding a session, and a mailed link may open in any browser.
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return <BrandedScreen>{children}</BrandedScreen>;
}
