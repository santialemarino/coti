-- Coti — datos de desarrollo. Idempotente: se puede correr varias veces.
--
-- Corre como owner, así que RLS no aplica. NO usar en producción.
-- Un corralón con dos sucursales: alcanza para ejercitar el catálogo de cuenta
-- con disponibilidad y precio por sucursal.

-- Contraseña de los dos usuarios: coti1234 (bcrypt, cost 10). Solo para desarrollo.

INSERT INTO account (id, name, legal_name, tax_id, brand_color) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'Corralón San Martín',
   'Corralón San Martín S.R.L.', '30-71234567-9', '#C2410C')
ON CONFLICT (id) DO NOTHING;

INSERT INTO branch (id, account_id, name, address, default_expiry_days) VALUES
  ('b0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001',
   'Villa Bosch', 'Av. Márquez 1520, Villa Bosch', 7),
  ('b0000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001',
   'Morón', 'Rivadavia 18400, Morón', 5)
ON CONFLICT (id) DO NOTHING;

INSERT INTO app_user (id, account_id, name, email, password_hash, role) VALUES
  ('c0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001',
   'Admin Dev', 'admin@corralonsanmartin.test',
   '$2a$10$S3vHxTMC/Pp5KLw4YAlyeOBduPSvE1Dh0D8ho0VNzjBXWrTjb.fJ2', 'ADMIN'),
  ('c0000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001',
   'Vendedor Dev', 'vendedor@corralonsanmartin.test',
   '$2a$10$S3vHxTMC/Pp5KLw4YAlyeOBduPSvE1Dh0D8ho0VNzjBXWrTjb.fJ2', 'SELLER')
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_branch (account_id, user_id, branch_id) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'c0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001'),
  ('a0000000-0000-4000-8000-000000000001', 'c0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000002'),
  ('a0000000-0000-4000-8000-000000000001', 'c0000000-0000-4000-8000-000000000002', 'b0000000-0000-4000-8000-000000000001')
ON CONFLICT (user_id, branch_id) DO NOTHING;

-- Un canal por tipo en la sucursal principal; carga manual también en Morón, que es lo
-- que toda sucursal necesita para originar un pedido de mostrador.
--
-- identifier queda NULL en todos: el seed no puede inventar un número de WhatsApp ni una
-- casilla reales, y si lo hiciera divergiría de las bases que ya venían migradas, donde
-- esas filas existen sin identificador y la restricción compuesta no las compara. Por
-- eso el ON CONFLICT apunta al índice parcial, que es el que sostiene la unicidad
-- mientras el identificador no esté.
INSERT INTO channel (account_id, branch_id, type) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', 'WHATSAPP'),
  ('a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', 'EMAIL'),
  ('a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', 'WEBAPP'),
  ('a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', 'MANUAL_ENTRY'),
  ('a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000002', 'MANUAL_ENTRY')
ON CONFLICT (branch_id, type) WHERE identifier IS NULL DO NOTHING;

-- Catálogo de cuenta. embedding queda NULL: lo puebla el pipeline de IA.
INSERT INTO product (id, account_id, code, canonical_name, description, unit, category) VALUES
  ('d0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'CEM-LN-50', 'Cemento Loma Negra 50 kg', 'Cemento portland normal CPN40', 'bolsa', 'Cementos'),
  ('d0000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'CEM-AVE-50', 'Cemento Avellaneda 50 kg', 'Cemento portland compuesto CPC40', 'bolsa', 'Cementos'),
  ('d0000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', 'CAL-HID-25', 'Cal hidratada 25 kg', 'Cal hidratada para revoques', 'bolsa', 'Cales'),
  ('d0000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', 'AREN-M3', 'Arena fina', 'Arena fina a granel', 'm3', 'Áridos'),
  ('d0000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', 'PIED-M3', 'Piedra partida 6-20', 'Piedra partida granítica', 'm3', 'Áridos'),
  ('d0000000-0000-4000-8000-000000000006', 'a0000000-0000-4000-8000-000000000001', 'LAD-HUE-12', 'Ladrillo hueco 12x18x33', 'Ladrillo cerámico hueco 8 huecos', 'unidad', 'Mampostería'),
  ('d0000000-0000-4000-8000-000000000007', 'a0000000-0000-4000-8000-000000000001', 'LAD-COM', 'Ladrillo común', 'Ladrillo macizo de campo', 'unidad', 'Mampostería'),
  ('d0000000-0000-4000-8000-000000000008', 'a0000000-0000-4000-8000-000000000001', 'HIE-8', 'Hierro aletado 8 mm', 'Barra conformada ADN 420, 12 m', 'barra', 'Hierros'),
  ('d0000000-0000-4000-8000-000000000009', 'a0000000-0000-4000-8000-000000000001', 'HIE-10', 'Hierro aletado 10 mm', 'Barra conformada ADN 420, 12 m', 'barra', 'Hierros'),
  ('d0000000-0000-4000-8000-000000000010', 'a0000000-0000-4000-8000-000000000001', 'MAL-Q188', 'Malla Q188 6x2,15', 'Malla soldada para contrapiso', 'panel', 'Hierros'),
  ('d0000000-0000-4000-8000-000000000011', 'a0000000-0000-4000-8000-000000000001', 'CHA-C25', 'Chapa acanalada C25 x 3 m', 'Chapa galvanizada sinusoidal', 'unidad', 'Chapas'),
  ('d0000000-0000-4000-8000-000000000012', 'a0000000-0000-4000-8000-000000000001', 'MEM-4MM', 'Membrana asfáltica 4 mm', 'Membrana con aluminio, rollo 10 m2', 'rollo', 'Impermeabilizantes'),
  ('d0000000-0000-4000-8000-000000000013', 'a0000000-0000-4000-8000-000000000001', 'PLAC-STD-12', 'Placa de yeso 12,5 mm estándar', 'Placa 1,20 x 2,40 m', 'placa', 'Construcción en seco'),
  ('d0000000-0000-4000-8000-000000000014', 'a0000000-0000-4000-8000-000000000001', 'PERF-MON-70', 'Perfil montante 70 mm', 'Perfil galvanizado 2,60 m', 'unidad', 'Construcción en seco'),
  ('d0000000-0000-4000-8000-000000000015', 'a0000000-0000-4000-8000-000000000001', 'PEG-INT-25', 'Pegamento para cerámicos 25 kg', 'Adhesivo interior', 'bolsa', 'Adhesivos'),
  ('d0000000-0000-4000-8000-000000000016', 'a0000000-0000-4000-8000-000000000001', 'CER-45', 'Cerámico 45x45 esmaltado', 'Cerámico para piso interior', 'm2', 'Revestimientos'),
  ('d0000000-0000-4000-8000-000000000017', 'a0000000-0000-4000-8000-000000000001', 'CAN-PVC-110', 'Caño PVC 110 mm x 4 m', 'Caño cloacal espiga-enchufe', 'unidad', 'Sanitarios'),
  ('d0000000-0000-4000-8000-000000000018', 'a0000000-0000-4000-8000-000000000001', 'CAN-TER-20', 'Caño termofusión 20 mm x 4 m', 'Caño para agua caliente', 'unidad', 'Sanitarios'),
  ('d0000000-0000-4000-8000-000000000019', 'a0000000-0000-4000-8000-000000000001', 'LAT-4-PIN', 'Látex interior 4 L', 'Pintura látex mate blanco', 'lata', 'Pinturas'),
  ('d0000000-0000-4000-8000-000000000020', 'a0000000-0000-4000-8000-000000000001', 'HID-20', 'Hidrófugo 20 kg', 'Aditivo hidrófugo para mezclas', 'balde', 'Impermeabilizantes')
ON CONFLICT (id) DO NOTHING;

-- Sinónimos coloquiales: lo que realmente escribe un cliente por WhatsApp.
INSERT INTO product_synonym (account_id, product_id, term, source) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001', 'portland', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001', 'loma negra', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001', 'bolsa de cemento', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000006', 'hueco del 12', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000008', 'hierro del 8', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000009', 'hierro del 10', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000013', 'durlock', 'seed'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000011', 'chapa acanalada', 'seed')
ON CONFLICT DO NOTHING;

-- Alternativas para el motor de recomendaciones.
INSERT INTO product_alternative (account_id, base_product_id, alternative_product_id, type) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000002', 'ECONOMY'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000001', 'PREMIUM'),
  ('a0000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000006', 'd0000000-0000-4000-8000-000000000007', 'EQUIVALENT')
ON CONFLICT (base_product_id, alternative_product_id) DO NOTHING;

-- Disponibilidad por sucursal: Villa Bosch tiene todo, Morón un subconjunto.
INSERT INTO branch_product (account_id, branch_id, product_id, stock)
SELECT 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', id, 250
FROM product WHERE account_id = 'a0000000-0000-4000-8000-000000000001'
ON CONFLICT (branch_id, product_id) DO NOTHING;

INSERT INTO branch_product (account_id, branch_id, product_id, stock)
SELECT 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000002', id, 60
FROM product
WHERE account_id = 'a0000000-0000-4000-8000-000000000001'
  AND category IN ('Cementos', 'Cales', 'Áridos', 'Mampostería', 'Hierros')
ON CONFLICT (branch_id, product_id) DO NOTHING;

-- Precio vigente por sucursal. Morón un 4% arriba.
INSERT INTO product_price (account_id, branch_id, product_id, price, min_price, valid_from)
SELECT bp.account_id, bp.branch_id, bp.product_id,
       v.price,
       round(v.price * 0.88, 2),
       now() - interval '30 days'
FROM branch_product bp
JOIN (VALUES
  ('d0000000-0000-4000-8000-000000000001'::uuid, 9500.00),
  ('d0000000-0000-4000-8000-000000000002'::uuid, 8900.00),
  ('d0000000-0000-4000-8000-000000000003'::uuid, 4200.00),
  ('d0000000-0000-4000-8000-000000000004'::uuid, 32000.00),
  ('d0000000-0000-4000-8000-000000000005'::uuid, 41000.00),
  ('d0000000-0000-4000-8000-000000000006'::uuid, 780.00),
  ('d0000000-0000-4000-8000-000000000007'::uuid, 320.00),
  ('d0000000-0000-4000-8000-000000000008'::uuid, 11800.00),
  ('d0000000-0000-4000-8000-000000000009'::uuid, 18400.00),
  ('d0000000-0000-4000-8000-000000000010'::uuid, 64000.00),
  ('d0000000-0000-4000-8000-000000000011'::uuid, 29500.00),
  ('d0000000-0000-4000-8000-000000000012'::uuid, 47000.00),
  ('d0000000-0000-4000-8000-000000000013'::uuid, 13900.00),
  ('d0000000-0000-4000-8000-000000000014'::uuid, 6800.00),
  ('d0000000-0000-4000-8000-000000000015'::uuid, 9200.00),
  ('d0000000-0000-4000-8000-000000000016'::uuid, 15600.00),
  ('d0000000-0000-4000-8000-000000000017'::uuid, 21500.00),
  ('d0000000-0000-4000-8000-000000000018'::uuid, 7400.00),
  ('d0000000-0000-4000-8000-000000000019'::uuid, 26800.00),
  ('d0000000-0000-4000-8000-000000000020'::uuid, 18900.00)
) AS v(product_id, price) ON v.product_id = bp.product_id
WHERE bp.branch_id = 'b0000000-0000-4000-8000-000000000001'
  AND NOT EXISTS (SELECT 1 FROM product_price pp WHERE pp.product_id = bp.product_id AND pp.branch_id = bp.branch_id);

INSERT INTO product_price (account_id, branch_id, product_id, price, min_price, valid_from)
SELECT pp.account_id, 'b0000000-0000-4000-8000-000000000002', pp.product_id,
       round(pp.price * 1.04, 2), round(pp.min_price * 1.04, 2), pp.valid_from
FROM product_price pp
JOIN branch_product bp
  ON bp.product_id = pp.product_id AND bp.branch_id = 'b0000000-0000-4000-8000-000000000002'
WHERE pp.branch_id = 'b0000000-0000-4000-8000-000000000001'
  AND NOT EXISTS (
    SELECT 1 FROM product_price x
    WHERE x.product_id = pp.product_id AND x.branch_id = 'b0000000-0000-4000-8000-000000000002');

-- Promos: una por cantidad escalonada, una sobre el total.
INSERT INTO promotion (id, account_id, branch_id, name, condition_type, action_type, action_value, priority, description) VALUES
  ('e0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', NULL,
   'Cemento por cantidad', 'QUANTITY_TIERED', 'PERCENTAGE', 0, 10,
   'Descuento escalonado sobre el cemento según la cantidad de bolsas'),
  ('e0000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', NULL,
   'Compra grande', 'ON_TOTAL', 'PERCENTAGE', 5, 1, '5% en compras sobre $500.000')
ON CONFLICT (id) DO NOTHING;

INSERT INTO promotion_condition_item (account_id, promotion_id, product_id, min_quantity) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001',
   'd0000000-0000-4000-8000-000000000001', 10)
ON CONFLICT DO NOTHING;

INSERT INTO promotion_tier (account_id, promotion_id, from_quantity, to_quantity, value) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001', 10, 49, 10),
  ('a0000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000001', 50, NULL, 15)
ON CONFLICT DO NOTHING;

INSERT INTO client (id, account_id, name, phone, origin_channel) VALUES
  ('f0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001',
   'Juan Pérez', '+5491155550001', 'WHATSAPP'),
  ('f0000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001',
   'Constructora del Oeste', '+5491155550002', 'EMAIL')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tag (account_id, name, color) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'Recurrente', '#16A34A'),
  ('a0000000-0000-4000-8000-000000000001', 'Obra grande', '#2563EB')
ON CONFLICT DO NOTHING;
