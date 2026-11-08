import { Skeleton } from '@repo/ui/components';

export default function ProfileLoading() {
  return (
    <main className="flex min-h-dvh flex-col items-center px-4 py-8">
      <div className="flex w-full max-w-3xl flex-col gap-y-6">
        <Skeleton className="w-40 h-8" />
        <Skeleton className="w-full h-80" />
      </div>
    </main>
  );
}
