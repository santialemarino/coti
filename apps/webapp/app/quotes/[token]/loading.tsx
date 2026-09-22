import { Skeleton } from '@repo/ui/components';

/*
 * The frame the public quote renders inside while its read suspends. It mirrors the page: the same
 * max-width column and the same block order, so the quote lands where the loading frame promised
 * it. Every placeholder carries a child — the line's own text — so it sizes to the real content
 * instead of a guessed width.
 */
export default function PublicQuoteLoading() {
  return (
    <main className="flex flex-col min-h-screen items-center px-4 py-8 sm:py-12">
      <div className="flex flex-col w-full max-w-5xl gap-y-6">
        <span aria-hidden="true" className="h-1 w-16 rounded-full bg-border" />
        <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
          <div className="flex flex-col min-w-0 gap-y-2">
            <Skeleton className="h-10 w-40 self-start" />
            <Skeleton className="w-1/3 text-heading-4">Corralón Centro</Skeleton>
            <Skeleton className="w-2/3 text-paragraph-sm">Av. Siempreviva 742</Skeleton>
          </div>
          <div className="flex flex-col items-start gap-y-1.5 sm:items-end">
            <Skeleton className="w-44 text-paragraph-medium">Cotización COT-000042</Skeleton>
            <Skeleton className="w-28 text-paragraph-sm">Versión 2</Skeleton>
            <Skeleton className="w-36 text-paragraph-sm">Emitida el 15 sep 2026</Skeleton>
          </div>
        </div>

        <div className="flex flex-col gap-y-1.5">
          <Skeleton className="w-1/2 text-heading-3">Hola, Obra Norte.</Skeleton>
          <Skeleton className="w-3/4 text-paragraph">
            Preparamos esta cotización para tu pedido.
          </Skeleton>
        </div>

        <section className="flex flex-col gap-y-4">
          <Skeleton className="w-24 text-heading-6">Materiales</Skeleton>
          {[0, 1].map((card) => (
            <div
              key={card}
              className="flex flex-col p-4 gap-y-3 bg-card border border-border rounded-1.5xl shadow-e1"
            >
              <Skeleton className="w-2/3 text-paragraph-medium">Cemento Portland 50kg</Skeleton>
              <Skeleton className="w-1/4 text-paragraph-xs">Código CEM-01</Skeleton>
              <Skeleton className="w-full text-paragraph-sm">
                Pediste: dos bolsas de cemento
              </Skeleton>
              <div className="grid grid-cols-3 gap-x-3">
                <Skeleton className="text-paragraph-sm">2 bolsa</Skeleton>
                <Skeleton className="text-paragraph-sm text-right">$ 110,00</Skeleton>
                <Skeleton className="text-paragraph-sm text-right">$ 220,00</Skeleton>
              </div>
            </div>
          ))}
        </section>

        <section className="flex flex-col p-4 gap-y-3 bg-card border border-border rounded-1.5xl shadow-e1">
          <div className="flex items-baseline justify-between gap-x-4">
            <Skeleton className="w-16 text-heading-6">Total</Skeleton>
            <Skeleton className="w-32 text-heading-4">$ 200,00</Skeleton>
          </div>
          <Skeleton className="w-1/2 text-paragraph-sm">Válida hasta el 22 sep 2026</Skeleton>
          <Skeleton className="w-2/3 text-paragraph-xs">Precios sujetos a disponibilidad.</Skeleton>
        </section>
      </div>
    </main>
  );
}
