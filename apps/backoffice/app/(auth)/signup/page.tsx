import { SignupForm } from '@/app/(auth)/signup/_components/signup-form';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('signup');

export default function SignupPage() {
  return <SignupForm />;
}
