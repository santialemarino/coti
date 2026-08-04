'use client';

import { useState } from 'react';
import { PlusIcon, SendIcon, Trash2Icon } from 'lucide-react';

import {
  Avatar,
  AvatarFallback,
  Badge,
  Button,
  Checkbox,
  Combobox,
  Hint,
  Input,
  Label,
  Pagination,
  Progress,
  RadioGroup,
  RadioGroupItem,
  SearchInput,
  Separator,
  Skeleton,
  Spinner,
  Switch,
  Textarea,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { Item, Row, Section } from '@/app/(protected)/design/_components/section';

const VARIANTS = ['default', 'secondary', 'outline', 'ghost', 'destructive', 'link'] as const;
const SIZES = ['xs', 'sm', 'default', 'lg'] as const;
const ICON_SIZES = ['icon-xs', 'icon-sm', 'icon', 'icon-lg'] as const;
const TONES = ['neutral', 'brand', 'success', 'warning', 'danger', 'solid', 'outline'] as const;

const BRANCHES = [
  { value: 'vb', label: 'Villa Bosch' },
  { value: 'mo', label: 'Morón' },
  { value: 'ro', label: 'Rosario', group: 'Interior' },
  { value: 'cb', label: 'Córdoba', group: 'Interior' },
  { value: 'mz', label: 'Mendoza', group: 'Interior' },
  { value: 'sa', label: 'Salta', group: 'Interior', disabled: true },
];

export function Controls() {
  const [branchA, setBranchA] = useState<string | null>(null);
  const [branchB, setBranchB] = useState<string | null>('vb');
  const [search, setSearch] = useState('Arena');
  const [mode, setMode] = useState('mail');
  const [filter, setFilter] = useState('todos');
  const [page, setPage] = useState(4);
  const [progress, setProgress] = useState(45);

  return (
    <>
      <Section title="Button" hint="6 variantes × 8 tamaños. Probá hover, foco de teclado y click.">
        {VARIANTS.map((variant) => (
          <Row key={variant}>
            <Item label={variant}>
              <Button variant={variant}>Cotizar</Button>
            </Item>
            <Item label="disabled">
              <Button variant={variant} disabled>
                Cotizar
              </Button>
            </Item>
            <Item label="con icono">
              <Button variant={variant}>
                <SendIcon />
                Enviar
              </Button>
            </Item>
            <Item label="cargando">
              <Button variant={variant} disabled>
                <Spinner size="sm" />
                Enviando…
              </Button>
            </Item>
          </Row>
        ))}
        <Separator />
        <Row>
          {SIZES.map((size) => (
            <Item key={size} label={size}>
              <Button size={size}>Cotizar</Button>
            </Item>
          ))}
          {ICON_SIZES.map((size) => (
            <Item key={size} label={size}>
              <Button size={size} aria-label="Agregar">
                <PlusIcon />
              </Button>
            </Item>
          ))}
        </Row>
      </Section>

      <Section title="Badge" hint="7 tonos × 2 tamaños, con o sin punto.">
        <Row>
          {TONES.map((tone) => (
            <Item key={tone} label={tone}>
              <Badge tone={tone}>{tone}</Badge>
            </Item>
          ))}
        </Row>
        <Row>
          {TONES.map((tone) => (
            <Item key={tone} label={`${tone} · dot`}>
              <Badge tone={tone} dot>
                {tone}
              </Badge>
            </Item>
          ))}
        </Row>
        <Row>
          <Item label="sm">
            <Badge size="sm">Sin match</Badge>
          </Item>
          <Item label="confianza alta">
            <Badge tone="success" dot>
              99%
            </Badge>
          </Item>
          <Item label="confianza media">
            <Badge tone="warning" dot>
              70%
            </Badge>
          </Item>
          <Item label="confianza baja">
            <Badge tone="danger" dot>
              10%
            </Badge>
          </Item>
        </Row>
      </Section>

      <Section title="Input" hint="Todas las combinaciones de slots y estados.">
        <div className="grid w-full grid-cols-1 gap-4 md:grid-cols-2">
          <Item label="base">
            <Input placeholder="tu@correo.com" />
          </Item>
          <Item label="password (probá el ojo)">
            <Input
              type="password"
              placeholder="Ingresá tu contraseña"
              passwordToggleLabel="Mostrar u ocultar la contraseña"
            />
          </Item>
          <Item label="aria-invalid">
            <Input aria-invalid placeholder="Inválido" />
          </Item>
          <Item label="readOnly">
            <Input readOnly value="No editable" />
          </Item>
          <Item label="disabled">
            <Input disabled placeholder="Deshabilitado" />
          </Item>
          <Item label="startIcon">
            <Input startIcon={<SendIcon className="size-4" />} placeholder="Con icono" />
          </Item>
          <Item label="prefix + suffix">
            <Input prefix="$" suffix="ARS" placeholder="0,00" />
          </Item>
          <Item label="endIcon">
            <Input endIcon={<Trash2Icon className="size-4" />} placeholder="Con acción" />
          </Item>
          <Item label="SearchInput (probá la X)">
            <SearchInput
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onClear={() => setSearch('')}
              clearLabel="Limpiar la búsqueda"
              placeholder="Buscá productos…"
            />
          </Item>
          <Item label="Textarea">
            <Textarea placeholder="Notas opcionales…" />
          </Item>
        </div>
        <Hint>Un Hint explica el formato o el valor por defecto de un campo.</Hint>
      </Section>

      <Section
        title="Combobox"
        hint="El único dropdown del sistema. Sin buscador escribí para saltar (type-ahead ciego); con buscador filtra."
      >
        <Row>
          <div className="w-72">
            <Item label="sin buscador">
              <Combobox
                options={BRANCHES}
                value={branchA}
                onValueChange={setBranchA}
                placeholder="Seleccioná una sucursal"
              />
            </Item>
          </div>
          <div className="w-72">
            <Item label="con buscador + grupos">
              <Combobox
                options={BRANCHES}
                value={branchB}
                onValueChange={setBranchB}
                placeholder="Seleccioná una sucursal"
                searchable
                searchPlaceholder="Buscá sucursal…"
                emptyLabel="Sin resultados"
              />
            </Item>
          </div>
          <div className="w-72">
            <Item label="aria-invalid">
              <Combobox
                options={BRANCHES}
                value={null}
                onValueChange={() => {}}
                placeholder="Seleccioná una sucursal"
                aria-invalid
              />
            </Item>
          </div>
        </Row>
      </Section>

      <Section title="Selección">
        <Row>
          <Item label="checkbox">
            <div className="flex items-center gap-x-2">
              <Checkbox id="d-c1" defaultChecked />
              <Label htmlFor="d-c1">Marcado</Label>
            </div>
          </Item>
          <Item label="sin marcar">
            <div className="flex items-center gap-x-2">
              <Checkbox id="d-c2" />
              <Label htmlFor="d-c2">Sin marcar</Label>
            </div>
          </Item>
          <Item label="indeterminate">
            <div className="flex items-center gap-x-2">
              <Checkbox id="d-c3" checked="indeterminate" />
              <Label htmlFor="d-c3">Parcial</Label>
            </div>
          </Item>
          <Item label="disabled">
            <div className="flex items-center gap-x-2">
              <Checkbox id="d-c4" disabled defaultChecked />
              <Label htmlFor="d-c4">Deshabilitado</Label>
            </div>
          </Item>
          <Item label="radio">
            <RadioGroup defaultValue="a" className="gap-y-2">
              <div className="flex items-center gap-x-2">
                <RadioGroupItem value="a" id="d-r1" />
                <Label htmlFor="d-r1">Opción A</Label>
              </div>
              <div className="flex items-center gap-x-2">
                <RadioGroupItem value="b" id="d-r2" />
                <Label htmlFor="d-r2">Opción B</Label>
              </div>
            </RadioGroup>
          </Item>
          <Item label="switch">
            <div className="flex items-center gap-x-2">
              <Switch id="d-s1" defaultChecked />
              <Label htmlFor="d-s1">Activado</Label>
            </div>
          </Item>
          <Item label="switch disabled">
            <Switch disabled />
          </Item>
          <Item label="required">
            <Label required>Campo obligatorio</Label>
          </Item>
        </Row>
      </Section>

      <Section title="ToggleGroup" hint="Segmentado para un modo, pills para un filtro.">
        <Row>
          <Item label="segmented">
            <ToggleGroup type="single" value={mode} onValueChange={(v) => v && setMode(v)}>
              <ToggleGroupItem value="mail">Mail</ToggleGroupItem>
              <ToggleGroupItem value="whatsapp">WhatsApp</ToggleGroupItem>
            </ToggleGroup>
          </Item>
          <Item label="segmented sm">
            <ToggleGroup type="single" size="sm" defaultValue="d">
              <ToggleGroupItem value="d">Día</ToggleGroupItem>
              <ToggleGroupItem value="s">Semana</ToggleGroupItem>
              <ToggleGroupItem value="m">Mes</ToggleGroupItem>
            </ToggleGroup>
          </Item>
          <Item label="pills">
            <ToggleGroup
              type="single"
              variant="pills"
              value={filter}
              onValueChange={(v) => v && setFilter(v)}
            >
              <ToggleGroupItem value="todos">Todos</ToggleGroupItem>
              <ToggleGroupItem value="abiertos">Abiertos</ToggleGroupItem>
              <ToggleGroupItem value="cerrados">Cerrados</ToggleGroupItem>
            </ToggleGroup>
          </Item>
        </Row>
      </Section>

      <Section title="Pagination">
        <div className="flex flex-col w-full gap-y-4">
          <Pagination
            page={page}
            pageCount={68}
            onPageChange={setPage}
            labels={{ previous: 'Anterior', next: 'Siguiente', page: 'Paginación' }}
          />
          <Pagination
            page={2}
            pageCount={4}
            onPageChange={() => {}}
            labels={{ previous: 'Anterior', next: 'Siguiente', page: 'Paginación' }}
          />
        </div>
      </Section>

      <Section title="Progress, Spinner, Avatar, Skeleton">
        <Row>
          <Item label="xs · sm · default · lg · xl">
            <div className="flex items-center gap-x-3">
              <Spinner size="xs" />
              <Spinner size="sm" />
              <Spinner />
              <Spinner size="lg" />
              <Spinner size="xl" />
            </div>
          </Item>
          <Item label="sm · default · lg">
            <div className="flex items-center gap-x-3">
              <Avatar size="sm">
                <AvatarFallback>SA</AvatarFallback>
              </Avatar>
              <Avatar>
                <AvatarFallback>VD</AvatarFallback>
              </Avatar>
              <Avatar size="lg">
                <AvatarFallback>AD</AvatarFallback>
              </Avatar>
            </div>
          </Item>
        </Row>
        <div className="flex flex-col w-full max-w-md gap-y-3">
          <Item label={`progress — ${progress}%`}>
            <div className="flex w-full flex-col gap-y-2">
              <Progress value={progress} label="Progreso" />
              <Progress value={progress} tone="success" size="sm" label="Progreso" />
              <Progress value={progress} tone="warning" size="lg" label="Progreso" />
              <Progress value={progress} tone="danger" label="Progreso" />
              <Progress value={progress} tone="neutral" label="Progreso" />
              <input
                type="range"
                min={0}
                max={100}
                value={progress}
                aria-label="Mover el progreso"
                onChange={(e) => setProgress(Number(e.target.value))}
              />
            </div>
          </Item>
          <Item label="skeleton (se dimensiona con su texto)">
            <div className="flex w-full flex-col gap-y-2">
              <Skeleton className="h-4">ID Pedido — Nombre Cliente</Skeleton>
              <Skeleton className="h-3">xx/xx/xx xxxx</Skeleton>
              <Skeleton className="size-10 rounded-full" />
            </div>
          </Item>
        </div>
      </Section>
    </>
  );
}
