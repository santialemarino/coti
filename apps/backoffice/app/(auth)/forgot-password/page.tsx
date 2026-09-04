import { ForgotPasswordForm } from '@/app/(auth)/forgot-password/_components/forgot-password-form';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('forgotPassword');

// The form owns its own card, because its sent state replaces the card rather than its contents.
export default function ForgotPasswordPage() {
  return <ForgotPasswordForm />;
}
