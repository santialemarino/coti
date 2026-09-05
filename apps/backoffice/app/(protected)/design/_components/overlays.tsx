'use client';

import { useState } from 'react';
import { ChevronDownIcon, KeyRoundIcon, LogOutIcon, PencilIcon, Trash2Icon } from 'lucide-react';
import { toast } from 'sonner';

import {
  Avatar,
  AvatarFallback,
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  ConfirmDialog,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DropdownChevron,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
  Textarea,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@repo/ui/components';
import { Item, Row, Section } from '@/app/(protected)/design/_components/section';

const SIDES = ['top', 'right', 'bottom', 'left'] as const;

export function Overlays() {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  async function confirm() {
    setPending(true);
    await new Promise((r) => setTimeout(r, 900));
    setPending(false);
    setConfirmOpen(false);
    toast.success('Se eliminó el pedido #123.');
  }

  return (
    <>
      <Section
        title="Overlays"
        hint="Abrí y cerrá cada uno: la salida está animada igual que la entrada. Escape también cierra."
      >
        <Row>
          <Item label="Dialog">
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="outline">Abrir dialog</Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Seleccionar método de envío</DialogTitle>
                  <DialogDescription>
                    Elegí cómo querés enviarle la cotización al cliente.
                  </DialogDescription>
                </DialogHeader>
                <Textarea placeholder="Mensaje que se le enviaría al cliente…" />
                <DialogFooter>
                  <Button variant="outline">Cancelar</Button>
                  <Button>Enviar</Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </Item>

          <Item label="Dialog · sin cerrar por fuera">
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="outline">Con datos sin guardar</Button>
              </DialogTrigger>
              <DialogContent closeOnClickOutside={false}>
                <DialogHeader>
                  <DialogTitle>No se cierra por click afuera</DialogTitle>
                  <DialogDescription>
                    Para un formulario a medio llenar: un click al costado no descarta lo escrito.
                    Escape y la ✕ sí cierran.
                  </DialogDescription>
                </DialogHeader>
              </DialogContent>
            </Dialog>
          </Item>

          <Item label="Popover">
            <Popover>
              <PopoverTrigger asChild>
                <Button variant="outline">Abrir popover</Button>
              </PopoverTrigger>
              <PopoverContent>
                <p className="text-paragraph-sm text-foreground-muted">
                  Entra desde el trigger y sale hacia él.
                </p>
              </PopoverContent>
            </Popover>
          </Item>

          <Item label="DropdownMenu">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="gap-x-2 pl-1.5">
                  <Avatar size="sm">
                    <AvatarFallback>AD</AvatarFallback>
                  </Avatar>
                  Perfil
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="min-w-52">
                <DropdownMenuLabel>Administrador</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  <DropdownMenuItem>
                    <KeyRoundIcon aria-hidden="true" />
                    Cambiar contraseña
                    <DropdownMenuShortcut>⌘K</DropdownMenuShortcut>
                  </DropdownMenuItem>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger>Sucursal</DropdownMenuSubTrigger>
                    <DropdownMenuSubContent>
                      <DropdownMenuItem>Villa Bosch</DropdownMenuItem>
                      <DropdownMenuItem>Morón</DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuItem tone="danger">
                  <LogOutIcon aria-hidden="true" />
                  Cerrar sesión
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </Item>

          <Item label="ConfirmDialog (con pending)">
            <Button variant="outline" onClick={() => setConfirmOpen(true)}>
              Eliminar pedido
            </Button>
          </Item>
        </Row>

        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          entity={{ name: 'Pedido #123' }}
          title="Eliminar pedido"
          description={(e) => `Se va a eliminar ${e.name}. Esta acción no se puede deshacer.`}
          onConfirm={confirm}
          pending={pending}
          labels={{ confirm: 'Eliminar', pending: 'Eliminando…', cancel: 'Cancelar' }}
        />
      </Section>

      <Section title="Sheet" hint="Entra en 300ms y sale en 200ms, a propósito.">
        <Row>
          {SIDES.map((side) => (
            <Item key={side} label={side}>
              <Sheet>
                <SheetTrigger asChild>
                  <Button variant="outline">{side}</Button>
                </SheetTrigger>
                <SheetContent side={side}>
                  <SheetHeader>
                    <SheetTitle>Filtros</SheetTitle>
                    <SheetDescription>Acotá el listado de pedidos.</SheetDescription>
                  </SheetHeader>
                  <SheetFooter>
                    <Button variant="outline">Limpiar</Button>
                    <Button>Aplicar</Button>
                  </SheetFooter>
                </SheetContent>
              </Sheet>
            </Item>
          ))}
        </Row>
      </Section>

      <Section title="Tooltip" hint="Los cuatro lados.">
        <Row>
          {SIDES.map((side) => (
            <Item key={side} label={side}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="outline">{side}</Button>
                </TooltipTrigger>
                <TooltipContent side={side}>Aparece y desaparece con animación</TooltipContent>
              </Tooltip>
            </Item>
          ))}
          <Item label="en un icono">
            <div className="flex gap-x-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon-sm" aria-label="Editar">
                    <PencilIcon />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Editar</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon-sm" aria-label="Eliminar">
                    <Trash2Icon />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Eliminar</TooltipContent>
              </Tooltip>
            </div>
          </Item>
        </Row>
      </Section>

      <Section title="Collapsible y chevron">
        <div className="flex flex-col w-full max-w-md gap-y-4">
          <Collapsible open={collapsed} onOpenChange={setCollapsed}>
            <CollapsibleTrigger asChild>
              <Button variant="ghost" size="sm">
                Ver detalle de descuentos
                <DropdownChevron open={collapsed} />
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <div className="flex flex-col pt-3 gap-y-1 text-paragraph-sm text-foreground-muted">
                <span>Descuento por volumen — 8%</span>
                <span>Descuento por cliente — 4%</span>
                <span>Descuento por pago contado — 2%</span>
              </div>
            </CollapsibleContent>
          </Collapsible>
          <Item label="chevron suelto (cerrado / abierto)">
            <div className="flex items-center gap-x-4">
              <ChevronDownIcon className="size-4 text-foreground-subtle" />
              <DropdownChevron open={false} />
              <DropdownChevron open />
            </div>
          </Item>
        </div>
      </Section>

      <Section title="Toast" hint="Feedback transitorio de algo que el usuario acaba de hacer.">
        <Row>
          <Button variant="outline" onClick={() => toast.success('Se actualizaron 128 productos.')}>
            success
          </Button>
          <Button
            variant="outline"
            onClick={() => toast.error('No pudimos comunicarnos con el servidor.')}
          >
            error
          </Button>
          <Button
            variant="outline"
            onClick={() => toast.warning('La moneda seleccionada no es convertible.')}
          >
            warning
          </Button>
          <Button variant="outline" onClick={() => toast.info('La cotización venció hace 2 días.')}>
            info
          </Button>
          <Button
            variant="outline"
            onClick={() =>
              toast.success('Cotización enviada.', {
                description: 'Se envió a constructorax@gmail.com por mail.',
              })
            }
          >
            con descripción
          </Button>
        </Row>
      </Section>
    </>
  );
}
