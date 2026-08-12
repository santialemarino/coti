/*
 * The RFQ domain has no backend endpoints yet, so this module owns the shapes the screens consume
 * and serves example data. The list goes through an async boundary (`fetchRfqs`) so the screens
 * render their loading and error states for real; swapping it for a GET later is a change of this
 * module alone, because the components only know these interfaces.
 */

// Native statuses the domain defines; labels are frontend i18n (see docs/internal/domain/estados.md).
export type RfqStatus =
  | 'RECEIVED'
  | 'GENERATED'
  | 'QUOTED'
  | 'SENT'
  | 'CHANGE_REQUESTED'
  | 'ACCEPTED'
  | 'REJECTED';

// The channel a request arrived through; an icon and an i18n label hang off each value.
export type RfqChannel = 'whatsapp' | 'email' | 'audio' | 'photo' | 'pdf' | 'excel' | 'link';

export type RfqPriority = 'high' | 'normal' | 'low';

export interface RfqRecord {
  id: string;
  client: string;
  createdAt: string;
  channel: RfqChannel;
  seller: string;
  branch: string;
  itemCount: number;
  /*
   * Decimal string, the wire format money travels as (NUMERIC(14,2)); currency is per-account.
   * Absent until the quote exists — an uncotized request has no amount to show.
   */
  total?: string;
  priority: RfqPriority;
  status: RfqStatus;
  /*
   * UI-only while the mock stands in for the backend: true while the AI is generating the quote
   * (status still GENERATED). A real API would derive it from a generation job id.
   */
  processing?: boolean;
  /*
   * Archivado is an orthogonal flag, not a status (see docs/internal/domain/estados.md): a quote
   * keeps its real status and simply gets marked archived.
   */
  archived?: boolean;
}

const RFQS: RfqRecord[] = [
  {
    id: '1048',
    client: 'Constructora Andina',
    createdAt: '2026-08-06T14:20:00.000Z',
    channel: 'whatsapp',
    seller: 'María López',
    branch: 'Sucursal Centro',
    itemCount: 28,
    priority: 'high',
    status: 'GENERATED',
    // The mock stands in for an in-flight AI generation, so the spinner is visible at load.
    processing: true,
  },
  {
    id: '1047',
    client: 'Ferretería El Tanque',
    createdAt: '2026-08-06T11:05:00.000Z',
    channel: 'email',
    seller: 'Juan Pérez',
    branch: 'Sucursal Norte',
    itemCount: 12,
    total: '87430.00',
    priority: 'normal',
    status: 'QUOTED',
  },
  {
    id: '1046',
    client: 'Obra Delia S.A.',
    createdAt: '2026-08-05T19:21:00.000Z',
    channel: 'audio',
    seller: 'Lucía Fernández',
    branch: 'Sucursal Oeste',
    itemCount: 45,
    total: '623410.75',
    priority: 'high',
    status: 'SENT',
  },
  {
    id: '1045',
    client: 'Materiales Don Pedro',
    createdAt: '2026-08-05T18:55:00.000Z',
    channel: 'pdf',
    seller: 'Carlos Gómez',
    branch: 'Sucursal Centro',
    itemCount: 8,
    total: '52100.25',
    priority: 'normal',
    status: 'ACCEPTED',
  },
  {
    id: '1044',
    client: 'Cooperativa La Unión',
    createdAt: '2026-08-05T18:30:00.000Z',
    channel: 'email',
    seller: 'Pedro Silva',
    branch: 'Sucursal Norte',
    itemCount: 22,
    priority: 'normal',
    status: 'CHANGE_REQUESTED',
  },
  {
    id: '1043',
    client: 'Albañilería Ruiz',
    createdAt: '2026-08-05T18:12:00.000Z',
    channel: 'whatsapp',
    seller: 'María López',
    branch: 'Sucursal Centro',
    itemCount: 6,
    priority: 'low',
    status: 'GENERATED',
  },
  {
    id: '1042',
    client: 'Hormigonera San José',
    createdAt: '2026-08-05T17:45:00.000Z',
    channel: 'photo',
    seller: 'Juan Pérez',
    branch: 'Sucursal Oeste',
    itemCount: 15,
    priority: 'high',
    status: 'RECEIVED',
  },
  {
    id: '1041',
    client: 'Obra Torre Mar del Plata',
    createdAt: '2026-08-05T16:38:00.000Z',
    channel: 'excel',
    seller: 'Lucía Fernández',
    branch: 'Sucursal Norte',
    itemCount: 61,
    total: '894600.00',
    priority: 'high',
    status: 'QUOTED',
  },
  {
    id: '1040',
    client: 'Cerámica El Sur',
    createdAt: '2026-08-05T15:10:00.000Z',
    channel: 'whatsapp',
    seller: 'Carlos Gómez',
    branch: 'Sucursal Centro',
    itemCount: 4,
    total: '18750.00',
    priority: 'low',
    status: 'ACCEPTED',
  },
  {
    id: '1039',
    client: 'Casa del Constructor',
    createdAt: '2026-08-05T13:55:00.000Z',
    channel: 'link',
    seller: 'Pedro Silva',
    branch: 'Sucursal Oeste',
    itemCount: 19,
    total: '98760.50',
    priority: 'normal',
    status: 'SENT',
  },
  {
    id: '1038',
    client: 'Materiales El Ombú',
    createdAt: '2026-08-05T12:22:00.000Z',
    channel: 'email',
    seller: 'María López',
    branch: 'Sucursal Norte',
    itemCount: 11,
    total: '63400.00',
    priority: 'normal',
    status: 'REJECTED',
  },
  {
    id: '1037',
    client: 'Constructora Núñez',
    createdAt: '2026-08-05T10:47:00.000Z',
    channel: 'whatsapp',
    seller: 'Juan Pérez',
    branch: 'Sucursal Centro',
    itemCount: 34,
    priority: 'high',
    status: 'GENERATED',
  },
  {
    id: '1036',
    client: 'Techos La Frontera',
    createdAt: '2026-08-04T19:30:00.000Z',
    channel: 'photo',
    seller: 'Lucía Fernández',
    branch: 'Sucursal Oeste',
    itemCount: 7,
    total: '38900.00',
    priority: 'low',
    status: 'QUOTED',
  },
  {
    id: '1035',
    client: 'Instaladores BM S.A.',
    createdAt: '2026-08-04T17:08:00.000Z',
    channel: 'pdf',
    seller: 'Carlos Gómez',
    branch: 'Sucursal Norte',
    itemCount: 23,
    total: '276540.25',
    priority: 'normal',
    status: 'ACCEPTED',
  },
  {
    id: '1034',
    client: 'Herrería La Colonia',
    createdAt: '2026-08-04T15:42:00.000Z',
    channel: 'whatsapp',
    seller: 'Pedro Silva',
    branch: 'Sucursal Centro',
    itemCount: 9,
    total: '48200.00',
    priority: 'normal',
    status: 'SENT',
  },
  {
    id: '1033',
    client: 'Pisos y Revestimientos ACA',
    createdAt: '2026-08-04T14:15:00.000Z',
    channel: 'excel',
    seller: 'María López',
    branch: 'Sucursal Oeste',
    itemCount: 52,
    priority: 'high',
    status: 'CHANGE_REQUESTED',
  },
  {
    id: '1032',
    client: 'Consorcio Barrio Norte',
    createdAt: '2026-08-04T11:59:00.000Z',
    channel: 'email',
    seller: 'Juan Pérez',
    branch: 'Sucursal Norte',
    itemCount: 5,
    priority: 'low',
    status: 'GENERATED',
  },
  {
    id: '1031',
    client: 'Constructora del Litoral',
    createdAt: '2026-08-03T20:12:00.000Z',
    channel: 'whatsapp',
    seller: 'Lucía Fernández',
    branch: 'Sucursal Centro',
    itemCount: 31,
    total: '402300.00',
    priority: 'high',
    status: 'QUOTED',
  },
  {
    id: '1030',
    client: 'Obra Escuela Nº 24',
    createdAt: '2026-08-03T18:05:00.000Z',
    channel: 'audio',
    seller: 'Carlos Gómez',
    branch: 'Sucursal Oeste',
    itemCount: 17,
    total: '154200.00',
    priority: 'normal',
    status: 'ACCEPTED',
  },
  {
    id: '1029',
    client: 'Ferretería Chacarita',
    createdAt: '2026-08-03T16:40:00.000Z',
    channel: 'whatsapp',
    seller: 'Pedro Silva',
    branch: 'Sucursal Norte',
    itemCount: 3,
    total: '9800.00',
    priority: 'low',
    status: 'SENT',
  },
  {
    id: '1028',
    client: 'Cementos del Sur',
    createdAt: '2026-08-03T14:22:00.000Z',
    channel: 'link',
    seller: 'María López',
    branch: 'Sucursal Centro',
    itemCount: 26,
    priority: 'normal',
    status: 'CHANGE_REQUESTED',
  },
  {
    id: '1027',
    client: 'Plomería Total',
    createdAt: '2026-08-02T19:48:00.000Z',
    channel: 'email',
    seller: 'Juan Pérez',
    branch: 'Sucursal Oeste',
    itemCount: 10,
    priority: 'normal',
    status: 'GENERATED',
  },
  {
    id: '1026',
    client: 'Electricidad Costa',
    createdAt: '2026-08-02T17:35:00.000Z',
    channel: 'whatsapp',
    seller: 'Lucía Fernández',
    branch: 'Sucursal Norte',
    itemCount: 14,
    total: '102300.00',
    priority: 'normal',
    status: 'QUOTED',
  },
  {
    id: '1025',
    client: 'Obra Municipal Roca',
    createdAt: '2026-08-02T13:20:00.000Z',
    channel: 'pdf',
    seller: 'Carlos Gómez',
    branch: 'Sucursal Centro',
    itemCount: 38,
    priority: 'high',
    status: 'RECEIVED',
  },
  {
    id: '1024',
    client: 'Materiales El Palomar',
    createdAt: '2026-08-01T18:50:00.000Z',
    channel: 'whatsapp',
    seller: 'Pedro Silva',
    branch: 'Sucursal Oeste',
    itemCount: 13,
    total: '84600.00',
    priority: 'normal',
    status: 'ACCEPTED',
  },
  {
    id: '1023',
    client: 'Clásica Construcciones',
    createdAt: '2026-08-01T16:14:00.000Z',
    channel: 'photo',
    seller: 'María López',
    branch: 'Sucursal Norte',
    itemCount: 6,
    total: '24500.00',
    priority: 'low',
    status: 'SENT',
  },
  {
    id: '1022',
    client: 'Almacén de la Construcción',
    createdAt: '2026-08-01T12:45:00.000Z',
    channel: 'email',
    seller: 'Juan Pérez',
    branch: 'Sucursal Centro',
    itemCount: 20,
    priority: 'normal',
    status: 'GENERATED',
  },
  {
    id: '1021',
    client: 'Torre Premium S.A.',
    createdAt: '2026-07-31T15:30:00.000Z',
    channel: 'excel',
    seller: 'Lucía Fernández',
    branch: 'Sucursal Oeste',
    itemCount: 55,
    priority: 'high',
    status: 'CHANGE_REQUESTED',
  },
  {
    id: '1020',
    client: 'Obra Puente Nuevo',
    createdAt: '2026-07-30T11:05:00.000Z',
    channel: 'whatsapp',
    seller: 'Carlos Gómez',
    branch: 'Sucursal Norte',
    itemCount: 29,
    total: '376800.00',
    priority: 'high',
    status: 'QUOTED',
  },
  {
    id: '1019',
    client: 'Ferretería Don Ramón',
    createdAt: '2026-07-28T18:25:00.000Z',
    channel: 'audio',
    seller: 'Pedro Silva',
    branch: 'Sucursal Centro',
    itemCount: 8,
    total: '41650.00',
    priority: 'normal',
    status: 'REJECTED',
  },
];

// Simulated latency so the list's loading state is exercised; a real GET replaces the body, not
// the shape. Copies are returned so an archive in the UI never mutates the module's data.
const LATENCY_MS = 450;

// The stand-in duration of an AI quote generation until the backend drives it; the dashboard shows
// the processing spinner for this long when the seller marks a pedido as QUOTED.
export const QUOTE_GENERATION_MS = 1200;

export async function fetchRfqs(): Promise<RfqRecord[]> {
  await new Promise((resolve) => setTimeout(resolve, LATENCY_MS));
  return RFQS.map((rfq) => ({ ...rfq }));
}
