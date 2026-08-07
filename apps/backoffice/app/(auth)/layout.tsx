import { BrandedScreen } from '@/components/branded-screen';

// The gate bounces a signed-in caller off these routes before they render, so
// this only carries the shared frame.
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return <BrandedScreen>{children}</BrandedScreen>;
}
