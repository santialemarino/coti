import { BrandedScreen } from '@/components/branded-screen';

// Only the shared frame. Most of these routes are signed-out-only and the gate bounces a
// signed-in caller before they render — verify-email is the exception, and reads the session
// itself, because signup lands on it holding one.
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return <BrandedScreen>{children}</BrandedScreen>;
}
