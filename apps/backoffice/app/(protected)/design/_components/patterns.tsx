'use client';

import { useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  CircleCheckIcon,
  CircleXIcon,
  MailCheckIcon,
  PackageIcon,
  PencilIcon,
  RotateCcwIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import {
  Badge,
  Button,
  Callout,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  EmptyState,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  Input,
  PendingButton,
  RowActionButton,
  SortableTableHead,
  StatusScreen,
  Stepper,
  Table,
  TableBody,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableRow,
  type SortOrder,
} from '@repo/ui/components';
import { Item, Row, Section } from '@/app/(protected)/design/_components/section';

const STEPS = [
  { id: 'received', label: 'Recibido', meta: '02/08/26' },
  { id: 'generated', label: 'Generado', meta: '02/08/26' },
  { id: 'quoted', label: 'Cotizado' },
  { id: 'sent', label: 'Enviado' },
  { id: 'closed', label: 'Cerrado' },
];

const ROWS = [
  { product: 'Arena', confidence: 99, tone: 'success', qty: '100 kg' },
  { product: 'Cemento Loma Negra 50kg', confidence: 70, tone: 'warning', qty: '20 u.' },
  { product: 'Hierro del 8', confidence: 10, tone: 'danger', qty: '12 u.' },
] as const;

/* Quantity stays a string, like the API's NUMERIC decimals — and it keeps the field typed for RHF. */
const schema = z.object({
  email: z.string().min(1, 'Ingresá tu correo.').email('Ingresá una dirección de correo válida.'),
  quantity: z
    .string()
    .min(1, 'Ingresá una cantidad.')
    .refine((v) => Number(v) > 0, 'La cantidad tiene que ser mayor a cero.'),
});

type Values = z.infer<typeof schema>;

export function Patterns() {
  const [stepIndex, setStepIndex] = useState(2);
  const [sortOrder, setSortOrder] = useState<SortOrder>('asc');
  const [sortBy, setSortBy] = useState<string | null>('product');
  /* Real rows rather than a visibility flag, so the row actions have something to act on. */
  const [rows, setRows] = useState<readonly (typeof ROWS)[number][]>(ROWS);
  const [editing, setEditing] = useState<string | null>(null);
  /* Bumping the key remounts the status screens, which replays their one-shot entrance. */
  const [replay, setReplay] = useState(0);

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', quantity: '' },
    mode: 'onSubmit',
    reValidateMode: 'onChange',
  });

  async function onSubmit() {
    await new Promise((r) => setTimeout(r, 700));
    form.setError('root', { message: 'El servidor rechazó el pedido. Intentá de nuevo.' });
  }

  return (
    <>
      <Section
        title="StatusScreen"
        hint="Sólo el icono anima; el resto entra con la pantalla que lo contiene. Tocá Reproducir para verlo otra vez."
      >
        <Button variant="outline" size="sm" onClick={() => setReplay((n) => n + 1)}>
          <RotateCcwIcon />
          Reproducir la animación
        </Button>
        <div key={replay} className="grid w-full grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
          <Card>
            <StatusScreen
              icon={MailCheckIcon}
              tone="info"
              title="Revisá tu correo"
              description="Si la dirección está registrada, va a recibir un enlace en unos minutos."
            >
              <Button variant="outline" size="sm">
                Reenviar
              </Button>
              <InlineLink href="#">Volver a iniciar sesión</InlineLink>
            </StatusScreen>
          </Card>
          <Card>
            <StatusScreen
              icon={CircleCheckIcon}
              tone="success"
              title="Listo"
              description="Ya podés entrar con tu contraseña nueva."
            >
              <InlineLink href="#">Iniciar sesión</InlineLink>
            </StatusScreen>
          </Card>
          <Card>
            <StatusScreen
              icon={TriangleAlertIcon}
              tone="warning"
              title="Quedaron ítems sin match"
              description="Revisá los 2 ítems marcados antes de cotizar."
            >
              <InlineLink href="#">Ver los ítems</InlineLink>
            </StatusScreen>
          </Card>
          <Card>
            <StatusScreen
              icon={CircleXIcon}
              tone="danger"
              title="El enlace no es válido"
              description="Ya se usó o venció. Pedí uno nuevo."
            >
              <InlineLink href="#">Pedir un enlace nuevo</InlineLink>
            </StatusScreen>
          </Card>
        </div>
      </Section>

      <Section title="Callout" hint="Un mensaje permanente sobre lo que hay en pantalla.">
        <div className="flex flex-col w-full gap-y-3">
          <Callout tone="info" title="Revisión humana">
            La IA propone, el backend valida y el vendedor aprueba. Nunca escribe sola.
          </Callout>
          <Callout tone="success">Se actualizaron 128 productos.</Callout>
          <Callout tone="warning">Hay 2 ítems sin match en el catálogo.</Callout>
          <Callout tone="danger" title="No pudimos guardar">
            Revisá tu conexión e intentá de nuevo.
          </Callout>
        </div>
      </Section>

      <Section title="Stepper" hint="El riel del ciclo de vida. Movete entre estados.">
        <div className="flex flex-col w-full gap-y-4">
          <Stepper steps={STEPS} currentIndex={stepIndex} />
          <Row>
            {STEPS.map((step, i) => (
              <Button
                key={step.id}
                variant={i === stepIndex ? 'default' : 'outline'}
                size="sm"
                onClick={() => setStepIndex(i)}
              >
                {step.label}
              </Button>
            ))}
          </Row>
        </div>
      </Section>

      <Section
        title="Card"
        hint="Con y sin interactive. Tabulá para ver el foco en la interactiva."
      >
        <Row>
          <Card className="w-72">
            <CardHeader>
              <CardTitle>Card estática</CardTitle>
              <CardDescription>Sin hover ni foco.</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-paragraph-sm text-foreground-muted">Contenido.</p>
            </CardContent>
            <CardFooter>
              <InlineLink href="#">Una acción →</InlineLink>
            </CardFooter>
          </Card>
          <Card interactive tabIndex={0} className="w-72">
            <CardHeader>
              <div className="flex items-center gap-x-3">
                <span className="grid size-9 place-items-center bg-accent rounded-lg text-primary">
                  <PackageIcon className="size-4" />
                </span>
                <div className="flex flex-col">
                  <CardTitle>5</CardTitle>
                  <CardDescription>Pedidos generados</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <InlineLink href="#">Ver pedidos generados →</InlineLink>
            </CardContent>
          </Card>
        </Row>
      </Section>

      <Section title="InlineLink">
        <Row>
          <Item label="brand">
            <InlineLink href="#">Volver a iniciar sesión</InlineLink>
          </Item>
          <Item label="muted">
            <InlineLink href="#" tone="muted">
              Descargar pedido original
            </InlineLink>
          </Item>
          <Item label="danger">
            <InlineLink href="#" tone="danger">
              Eliminar la cuenta
            </InlineLink>
          </Item>
        </Row>
      </Section>

      <Section
        title="Table"
        hint="Ordená por Producto, y probá el estado vacío. El foco del header vive en el icono."
      >
        <Card className="w-full">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-y-1">
                <CardTitle>Detalles de pedido</CardTitle>
                <CardDescription>
                  Con orden, badges de confianza y acciones por fila.
                </CardDescription>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setRows((current) => (current.length ? [] : ROWS))}
              >
                {rows.length ? 'Vaciar la tabla' : 'Mostrar filas'}
              </Button>
            </div>
          </CardHeader>
          <CardContent className="px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <SortableTableHead
                    label="Producto"
                    column="product"
                    sortBy={sortBy}
                    sortOrder={sortOrder}
                    onSort={(c) => {
                      if (c === sortBy) setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
                      else setSortBy(c);
                    }}
                  />
                  <TableHead>Confianza</TableHead>
                  <TableHead>Cantidad</TableHead>
                  <TableHead className="text-right">Acciones</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.length === 0 ? (
                  <TableEmptyRow
                    colSpan={4}
                    icon={PackageIcon}
                    title="Todavía no hay ítems"
                    description="Cuando llegue una solicitud, los ítems extraídos aparecen acá."
                  />
                ) : (
                  rows.map((row) => (
                    <TableRow key={row.product}>
                      <TableCell>
                        <span className="flex items-center gap-x-2">
                          {row.product}
                          {editing === row.product ? <Badge size="sm">Editando</Badge> : null}
                        </span>
                      </TableCell>
                      <TableCell>
                        <Badge tone={row.tone} dot>
                          {row.confidence}%
                        </Badge>
                      </TableCell>
                      <TableCell>{row.qty}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-x-1">
                          <RowActionButton
                            icon={PencilIcon}
                            label="Editar"
                            onClick={() =>
                              setEditing((current) =>
                                current === row.product ? null : row.product,
                              )
                            }
                          />
                          <RowActionButton
                            icon={Trash2Icon}
                            label="Eliminar"
                            tone="danger"
                            onClick={() =>
                              setRows((current) => current.filter((r) => r.product !== row.product))
                            }
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </Section>

      <Section title="EmptyState" hint="Donde iría el contenido cuando no hay ninguno.">
        <Card className="w-80">
          <EmptyState
            icon={PackageIcon}
            title="Todavía no hay pedidos"
            description="Cuando llegue una solicitud por WhatsApp o mail, la vas a ver acá."
          >
            <Button size="sm">Crear pedido</Button>
          </EmptyState>
        </Card>
      </Section>

      <Section
        title="Form"
        hint="Enviá vacío para ver los errores revelarse sin mover el layout, y corregí para verlos colapsar. El error de formulario aparece abajo."
      >
        <Card className="w-full max-w-md">
          <CardContent>
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(onSubmit)}
                noValidate
                className="flex flex-col gap-y-4"
              >
                <FormField
                  control={form.control}
                  name="email"
                  render={({ field }) => (
                    <FormItem required>
                      <FormLabel>Correo electrónico</FormLabel>
                      <FormControl>
                        <Input type="email" placeholder="tu@correo.com" {...field} />
                      </FormControl>
                      <FormDescription>Le mandamos la cotización a esta dirección.</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="quantity"
                  render={({ field }) => (
                    <FormItem required>
                      <FormLabel>Cantidad</FormLabel>
                      <FormControl>
                        <Input type="number" suffix="kg" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormRootMessage />
                <PendingButton
                  type="submit"
                  size="lg"
                  pending={form.formState.isSubmitting}
                  pendingLabel="Enviando…"
                >
                  Enviar
                </PendingButton>
              </form>
            </Form>
          </CardContent>
        </Card>
      </Section>
    </>
  );
}
