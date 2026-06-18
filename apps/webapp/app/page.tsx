import { Button } from '@repo/ui/components';

export default function Home() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6">
      <h1 className="text-3xl font-bold">Coti</h1>
      <Button>View quote</Button>
    </main>
  );
}
