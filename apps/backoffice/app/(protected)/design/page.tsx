import type { Metadata } from 'next';

import { Callout } from '@repo/ui/components';
import { Controls } from '@/app/(protected)/design/_components/controls';
import { Foundations } from '@/app/(protected)/design/_components/foundations';
import { Overlays } from '@/app/(protected)/design/_components/overlays';
import { Patterns } from '@/app/(protected)/design/_components/patterns';

/*
 * TEMPORARY — delete this whole folder when it stops being useful (`rm -rf app/(protected)/design`).
 * It is a workbench for trying the design system, not product surface, which is why it breaks two
 * conventions on purpose: its copy is hardcoded rather than going through next-intl, and its route is
 * not registered in config/routes.ts. Nothing links to it; reach it at /design. Do not copy these two
 * exemptions into a real page.
 */
export const metadata: Metadata = {
  title: 'Design system — Coti',
  robots: { index: false, follow: false },
};

export default function DesignPage() {
  return (
    <main className="flex flex-col mx-auto w-full max-w-6xl px-6 py-10 gap-y-12">
      <div className="flex flex-col gap-y-3">
        <h1 className="text-heading-1 text-foreground">Design system</h1>
        <p className="text-paragraph text-foreground-muted">
          Cada componente con sus variantes y sus cuatro estados. Tabulá con el teclado para ver el
          foco, y abrí y cerrá los overlays para ver que la salida también está animada.
        </p>
        <Callout tone="info" title="Página temporal">
          Es un banco de pruebas, no una pantalla del producto. Para probar el modo de movimiento
          reducido, activá &ldquo;Reducir movimiento&rdquo; en Accesibilidad del sistema y recargá:
          la animación decorativa desaparece y el foco sigue leyéndose.
        </Callout>
      </div>

      <Foundations />
      <Controls />
      <Overlays />
      <Patterns />
    </main>
  );
}
