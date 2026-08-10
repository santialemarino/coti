/*
 * The catalog has no frontend endpoint wired yet, so this module owns the shape the manual order
 * builder consumes and serves example data. The search goes through an async boundary
 * (`searchCatalog`) so the screen exercises a loading state; swapping it for the real GET is a
 * change of this module alone, because the components only know this interface.
 */

export interface CatalogProduct {
  id: string;
  code: string;
  name: string;
  /* Free text (bolsa, m2, kg, barra, ...), mirroring the backend's nullable `unit`. */
  unit: string;
  category: string;
  // Decimal string, the wire format money travels as (NUMERIC(14,2)); the price is per-branch.
  price: string;
}

const CATALOG: CatalogProduct[] = [
  {
    id: 'p1',
    code: 'CEM-P50',
    name: 'Cemento Portland 50 kg',
    unit: 'bolsa',
    category: 'Hormigones',
    price: '18500.00',
  },
  {
    id: 'p2',
    code: 'CAL-H25',
    name: 'Cal Hidratada 25 kg',
    unit: 'bolsa',
    category: 'Hormigones',
    price: '8400.00',
  },
  {
    id: 'p3',
    code: 'HIE-8',
    name: 'Hierro 8 mm x 12 m',
    unit: 'barra',
    category: 'Herrería',
    price: '21800.00',
  },
  {
    id: 'p4',
    code: 'HIE-6',
    name: 'Hierro 6 mm x 12 m',
    unit: 'barra',
    category: 'Herrería',
    price: '12300.00',
  },
  {
    id: 'p5',
    code: 'LAD-COM',
    name: 'Ladrillo Común',
    unit: 'millar',
    category: 'Mampostería',
    price: '285000.00',
  },
  {
    id: 'p6',
    code: 'LAD-HUE',
    name: 'Ladrillo Hueco 12x18x33',
    unit: 'unidad',
    category: 'Mampostería',
    price: '950.00',
  },
  {
    id: 'p7',
    code: 'ARE-FIN',
    name: 'Arena Fina',
    unit: 'm3',
    category: 'Áridos',
    price: '42000.00',
  },
  {
    id: 'p8',
    code: 'ARE-GRU',
    name: 'Arena Gruesa',
    unit: 'm3',
    category: 'Áridos',
    price: '38500.00',
  },
  {
    id: 'p9',
    code: 'PIE-620',
    name: 'Piedra Partida 6/20',
    unit: 'm3',
    category: 'Áridos',
    price: '55000.00',
  },
  {
    id: 'p10',
    code: 'PEG-CER',
    name: 'Pegamento Cerámico 25 kg',
    unit: 'bolsa',
    category: 'Revestimientos',
    price: '14200.00',
  },
  {
    id: 'p11',
    code: 'PAS-NIV',
    name: 'Pasta Niveladora 20 kg',
    unit: 'bolsa',
    category: 'Revestimientos',
    price: '12900.00',
  },
  {
    id: 'p12',
    code: 'CIN-PAP',
    name: 'Cinta Adhesiva Papel 48 mm x 50 m',
    unit: 'unidad',
    category: 'Revestimientos',
    price: '3800.00',
  },
  {
    id: 'p13',
    code: 'CAB-215',
    name: 'Cable 2x1,5 mm',
    unit: 'metro',
    category: 'Electricidad',
    price: '1250.00',
  },
  {
    id: 'p14',
    code: 'PIN-LAT',
    name: 'Pintura Látex Interior 10 L',
    unit: 'tarro',
    category: 'Pinturería',
    price: '48000.00',
  },
];

const LATENCY_MS = 220;

export async function searchCatalog(query: string): Promise<CatalogProduct[]> {
  await new Promise((resolve) => setTimeout(resolve, LATENCY_MS));
  const q = query.trim().toLowerCase();
  if (!q) return CATALOG.map((product) => ({ ...product }));
  return CATALOG.filter(
    (product) =>
      product.name.toLowerCase().includes(q) ||
      product.code.toLowerCase().includes(q) ||
      product.category.toLowerCase().includes(q),
  ).map((product) => ({ ...product }));
}
