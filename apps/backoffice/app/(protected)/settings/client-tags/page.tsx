import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ClientTagRuleForm } from '@/app/(protected)/settings/client-tags/_components/client-tag-rule-form';
import { apiRequest } from '@/lib/api/client';
import { mapClientTagRule, type ClientTagRuleRaw } from '@/lib/api/client-tag-rules';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clientTagSettings');

export default async function ClientTagSettingsPage() {
  await requireAdmin();
  const [t, rules] = await Promise.all([
    getTranslations('clientTagSettings'),
    apiRequest<{ items: ClientTagRuleRaw[] }>({ path: '/v1/tags/rules', branchScoped: false }),
  ]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <div className="grid gap-4 lg:grid-cols-2">
        {rules.items.map((rule) => (
          <ClientTagRuleForm key={rule.kind} rule={mapClientTagRule(rule)} />
        ))}
      </div>
    </main>
  );
}
