import type { CustomerAuthMode } from '@/components/customer-auth-schema';
import { CustomerAuthScreen } from '@/components/customer-auth-screen';
import { getCustomerAuthOptions } from '@/lib/api/customer-auth-options';
import { safeCustomerDestination } from '@/lib/auth/customer-flow';

interface CustomerAuthPageProps {
  mode: CustomerAuthMode;
  next: string;
  taxId?: string;
  googleEmail?: string;
  initialError?: string;
}

export async function CustomerAuthPage(props: CustomerAuthPageProps) {
  const options = await getCustomerAuthOptions();
  return (
    <CustomerAuthScreen
      {...props}
      next={safeCustomerDestination(props.next)}
      supplierName={options.supplierName}
      logoUrl={options.logoUrl}
      minimum={options.passwordMinLength}
      googleEnabled={Boolean(options.googleClientId)}
    />
  );
}
