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
   'Constructora del Oeste', '+5491155550002', 'EMAIL'),
  ('f0000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001',
   'Roberto Gómez', '+5491155550003', 'WALK_IN')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tag (account_id, name, color) VALUES
  ('a0000000-0000-4000-8000-000000000001', 'Recurrente', '#16A34A'),
  ('a0000000-0000-4000-8000-000000000001', 'Obra grande', '#2563EB')
ON CONFLICT DO NOTHING;

-- Combos de catálogo: de la cuenta, con disponibilidad por sucursal. El de contrapiso está
-- inactivo en Morón para que la disponibilidad no sea siempre TRUE, y el de revoque no tiene
-- fila en Morón — la ausencia de fila y el activo en FALSE son dos casos distintos.
INSERT INTO combo (id, account_id, name, description) VALUES
  ('70000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001',
   'Combo contrapiso 20 m2', 'Cemento, arena, piedra y malla para 20 m2 de contrapiso'),
  ('70000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001',
   'Combo revoque 30 m2', 'Cal, cemento y arena para revoque grueso de 30 m2')
ON CONFLICT (id) DO NOTHING;

INSERT INTO combo_item (id, account_id, combo_id, product_id, quantity) VALUES
  ('80000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001', 8),
  ('80000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000004', 2),
  ('80000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000005', 3),
  ('80000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000010', 4),
  ('80000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000003', 6),
  ('80000000-0000-4000-8000-000000000006', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000001', 3),
  ('80000000-0000-4000-8000-000000000007', 'a0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000004', 1.5)
ON CONFLICT (id) DO NOTHING;

-- Conflicto por la clave natural y no por el id: la migración que crea branch_combo puebla
-- la disponibilidad desde combo con ids nuevos, así que después de un down y un up los ids
-- del seed ya no existen y un ON CONFLICT (id) dejaría pasar la fila hasta chocar contra
-- uq_branch_combo.
INSERT INTO branch_combo (id, account_id, branch_id, combo_id, is_active) VALUES
  ('90000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000001', TRUE),
  ('90000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000002', '70000000-0000-4000-8000-000000000001', FALSE),
  ('90000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001', '70000000-0000-4000-8000-000000000002', TRUE)
ON CONFLICT (branch_id, combo_id) DO NOTHING;

-- =============================================================================
-- PEDIDOS Y COTIZACIONES
-- =============================================================================
--
-- Cinco cotizaciones en estados distintos, para que la bandeja del vendedor tenga algo que
-- mostrar sin que exista todavía el pipeline de IA. No las escribe un service: son SQL como
-- el resto del seed, así que las invariantes que el service haría cumplir van escritas a
-- mano acá. Las que importan:
--
--   * `quote_version.total` = Σ subtotales − Σ descuentos. Sin descuentos sembrados, es la
--     suma de los subtotales, y los ítems sin match no suman (su subtotal es NULL).
--   * Una sola versión sin congelar por cotización (`uq_quote_version_draft`). Enviada ⇒ la
--     versión que salió queda congelada; el pedido de cambio abre una v2 sin congelar.
--   * `current_status` se setea explícito y cada transición deja su fila en
--     `quote_status_change`, con `previous_status` NULL en la primera.
--   * Los precios son los de la sucursal: Villa Bosch al valor de `product_price`, Morón un
--     4% arriba, y `min_price_snapshot` al 88% del precio, igual que el seed de precios.
--
-- `channel_id` se resuelve por subconsulta y no por UUID fijo: los canales se insertan sin id
-- explícito, así que en una base que ya venía sembrada tienen otros ids que en una nueva.
--
-- Deliberadamente sin sembrar: descuentos aplicados, mensajes y acciones del cliente. La
-- cotización en CHANGE_REQUESTED, entonces, no tiene la acción que pidió el cambio.

INSERT INTO rfq (id, account_id, branch_id, client_id, channel_id, raw_text, status, work_type, received_at) VALUES
  ('10000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000001',
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000001' AND type = 'WHATSAPP' AND identifier IS NULL),
   'hola, necesito 50 bolsas de portland, 1000 huecos del 12 y 10 hierros del 8 para una losa. me pasás precio?',
   'GENERATED', 'Losa', now() - interval '3 hours'),
  ('10000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000002',
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000001' AND type = 'EMAIL' AND identifier IS NULL),
   'Buenos días, adjunto el pedido de la obra de Ramos Mejía: 6 m3 de arena fina, 8 m3 de piedra 6-20, 12 mallas Q188 y algo para impermeabilizar el contrapiso.',
   'GENERATED', 'Contrapiso', now() - interval '2 days'),
  ('10000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000002',
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000001' AND type = 'WEBAPP' AND identifier IS NULL),
   '40 placas de yeso de 12,5 y 60 montantes de 70 para un tabique.',
   'GENERATED', 'Construcción en seco', now() - interval '5 days'),
  ('10000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000001',
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000001' AND type = 'WHATSAPP' AND identifier IS NULL),
   'necesito 30 bolsas de cemento y 20 de cal para revoque',
   'GENERATED', 'Revoque', now() - interval '8 days'),
  ('10000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000002',
   NULL,
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000002' AND type = 'MANUAL_ENTRY' AND identifier IS NULL),
   'Mostrador: 10 bolsas de cemento y 500 ladrillos huecos del 12. Cliente sin datos, retira mañana.',
   'GENERATED', NULL, now() - interval '40 minutes')
ON CONFLICT (id) DO NOTHING;

INSERT INTO rfq_status_change (id, account_id, rfq_id, previous_status, new_status, user_id, changed_at) VALUES
  ('50000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001', 'RECEIVED', 'GENERATED', NULL, now() - interval '3 hours'),
  ('50000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000002', 'RECEIVED', 'GENERATED', NULL, now() - interval '2 days'),
  ('50000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000003', 'RECEIVED', 'GENERATED', NULL, now() - interval '5 days'),
  ('50000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000004', 'RECEIVED', 'GENERATED', NULL, now() - interval '8 days'),
  ('50000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000005', 'RECEIVED', 'GENERATED', 'c0000000-0000-4000-8000-000000000001', now() - interval '40 minutes')
ON CONFLICT (id) DO NOTHING;

-- current_version_id arranca NULL y se completa más abajo: quote y quote_version se
-- referencian mutuamente, así que no hay orden de inserción que satisfaga las dos claves.
INSERT INTO quote (id, account_id, branch_id, client_id, rfq_id, seller_id, current_status, expires_at, created_at) VALUES
  ('20000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001', NULL, 'DRAFT', NULL, now() - interval '3 hours'),
  ('20000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000002', '10000000-0000-4000-8000-000000000002', 'c0000000-0000-4000-8000-000000000002', 'QUOTED', now() + interval '5 days', now() - interval '2 days'),
  ('20000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000002', '10000000-0000-4000-8000-000000000003', 'c0000000-0000-4000-8000-000000000002', 'SENT', now() + interval '2 days', now() - interval '5 days'),
  ('20000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
   'f0000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000004', 'c0000000-0000-4000-8000-000000000002', 'CHANGE_REQUESTED', now() - interval '1 day', now() - interval '8 days'),
  ('20000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000002',
   NULL, '10000000-0000-4000-8000-000000000005', 'c0000000-0000-4000-8000-000000000001', 'DRAFT', NULL, now() - interval '40 minutes')
ON CONFLICT (id) DO NOTHING;

INSERT INTO quote_version (id, account_id, quote_id, author_id, version_number, total, is_immutable, comment, created_at) VALUES
  ('30000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000001', NULL, 1, 1373000.00, FALSE, NULL, now() - interval '3 hours'),
  ('30000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000002', 'c0000000-0000-4000-8000-000000000002', 1, 1288000.00, FALSE, 'Falta definir el impermeabilizante con el cliente', now() - interval '2 days'),
  ('30000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000003', 'c0000000-0000-4000-8000-000000000002', 1, 964000.00, TRUE, NULL, now() - interval '5 days'),
  ('30000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000004', 'c0000000-0000-4000-8000-000000000002', 1, 369000.00, TRUE, NULL, now() - interval '8 days'),
  ('30000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000004', 'c0000000-0000-4000-8000-000000000002', 2, 539600.00, FALSE, 'El cliente sumó 10 bolsas de cemento y pidió hidrófugo', now() - interval '1 day'),
  ('30000000-0000-4000-8000-000000000006', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000005', 'c0000000-0000-4000-8000-000000000001', 1, 504400.00, FALSE, NULL, now() - interval '40 minutes')
ON CONFLICT (id) DO NOTHING;

UPDATE quote SET current_version_id = v.version_id
FROM (VALUES
  ('20000000-0000-4000-8000-000000000001'::uuid, '30000000-0000-4000-8000-000000000001'::uuid),
  ('20000000-0000-4000-8000-000000000002'::uuid, '30000000-0000-4000-8000-000000000002'::uuid),
  ('20000000-0000-4000-8000-000000000003'::uuid, '30000000-0000-4000-8000-000000000003'::uuid),
  ('20000000-0000-4000-8000-000000000004'::uuid, '30000000-0000-4000-8000-000000000005'::uuid),
  ('20000000-0000-4000-8000-000000000005'::uuid, '30000000-0000-4000-8000-000000000006'::uuid)
) AS v(quote_id, version_id)
WHERE quote.id = v.quote_id AND quote.current_version_id IS NULL;

-- El ítem sin match va con product_id, precio y subtotal en NULL y match_status NO_MATCH:
-- se flaggea, nunca se descarta.
INSERT INTO quote_item (id, account_id, version_id, product_id, requested_description, quantity, unit, unit_price_snapshot, min_price_snapshot, subtotal, confidence_score, match_status) VALUES
  ('40000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000001', '50 bolsas de portland', 50, 'bolsa', 9500.00, 8360.00, 475000.00, 0.9600, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000002', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000006', '1000 huecos del 12', 1000, 'unidad', 780.00, 686.40, 780000.00, 0.9400, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000003', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000001', 'd0000000-0000-4000-8000-000000000008', '10 hierros del 8', 10, 'barra', 11800.00, 10384.00, 118000.00, 0.9800, 'MATCHED'),

  ('40000000-0000-4000-8000-000000000004', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000004', '6 m3 de arena fina', 6, 'm3', 32000.00, 28160.00, 192000.00, 0.9100, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000005', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000005', '8 m3 de piedra 6-20', 8, 'm3', 41000.00, 36080.00, 328000.00, 0.9300, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000006', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000002', 'd0000000-0000-4000-8000-000000000010', '12 mallas Q188', 12, 'panel', 64000.00, 56320.00, 768000.00, 0.9700, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000007', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000002', NULL, 'algo para impermeabilizar el contrapiso', 1, NULL, NULL, NULL, NULL, NULL, 'NO_MATCH'),

  ('40000000-0000-4000-8000-000000000008', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000003', 'd0000000-0000-4000-8000-000000000013', '40 placas de yeso de 12,5', 40, 'placa', 13900.00, 12232.00, 556000.00, 0.9500, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000009', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000003', 'd0000000-0000-4000-8000-000000000014', '60 montantes de 70', 60, 'unidad', 6800.00, 5984.00, 408000.00, 0.9200, 'MATCHED'),

  ('40000000-0000-4000-8000-000000000010', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000004', 'd0000000-0000-4000-8000-000000000001', '30 bolsas de cemento', 30, 'bolsa', 9500.00, 8360.00, 285000.00, 0.9600, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000011', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000004', 'd0000000-0000-4000-8000-000000000003', '20 de cal', 20, 'bolsa', 4200.00, 3696.00, 84000.00, 0.9000, 'MATCHED'),

  ('40000000-0000-4000-8000-000000000012', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000005', 'd0000000-0000-4000-8000-000000000001', '40 bolsas de cemento', 40, 'bolsa', 9500.00, 8360.00, 380000.00, 0.9600, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000013', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000005', 'd0000000-0000-4000-8000-000000000003', '20 de cal', 20, 'bolsa', 4200.00, 3696.00, 84000.00, 0.9000, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000014', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000005', 'd0000000-0000-4000-8000-000000000020', 'hidrófugo', 4, 'balde', 18900.00, 16632.00, 75600.00, 0.8900, 'MATCHED'),

  ('40000000-0000-4000-8000-000000000015', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000006', 'd0000000-0000-4000-8000-000000000001', '10 bolsas de cemento', 10, 'bolsa', 9880.00, 8694.40, 98800.00, 1.0000, 'MATCHED'),
  ('40000000-0000-4000-8000-000000000016', 'a0000000-0000-4000-8000-000000000001', '30000000-0000-4000-8000-000000000006', 'd0000000-0000-4000-8000-000000000006', '500 ladrillos huecos del 12', 500, 'unidad', 811.20, 713.86, 405600.00, 1.0000, 'MATCHED')
ON CONFLICT (id) DO NOTHING;

INSERT INTO quote_status_change (id, account_id, quote_id, previous_status, new_status, user_id, changed_at) VALUES
  ('50000000-0000-4000-8000-000000000011', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000001', NULL, 'DRAFT', NULL, now() - interval '3 hours'),

  ('50000000-0000-4000-8000-000000000012', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000002', NULL, 'DRAFT', NULL, now() - interval '2 days'),
  ('50000000-0000-4000-8000-000000000013', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000002', 'DRAFT', 'QUOTED', 'c0000000-0000-4000-8000-000000000002', now() - interval '2 days' + interval '20 minutes'),

  ('50000000-0000-4000-8000-000000000014', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000003', NULL, 'DRAFT', NULL, now() - interval '5 days'),
  ('50000000-0000-4000-8000-000000000015', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000003', 'DRAFT', 'QUOTED', 'c0000000-0000-4000-8000-000000000002', now() - interval '5 days' + interval '35 minutes'),
  ('50000000-0000-4000-8000-000000000016', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000003', 'QUOTED', 'SENT', 'c0000000-0000-4000-8000-000000000002', now() - interval '5 days' + interval '1 hour'),

  ('50000000-0000-4000-8000-000000000017', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000004', NULL, 'DRAFT', NULL, now() - interval '8 days'),
  ('50000000-0000-4000-8000-000000000018', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000004', 'DRAFT', 'QUOTED', 'c0000000-0000-4000-8000-000000000002', now() - interval '8 days' + interval '15 minutes'),
  ('50000000-0000-4000-8000-000000000019', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000004', 'QUOTED', 'SENT', 'c0000000-0000-4000-8000-000000000002', now() - interval '7 days'),
  ('50000000-0000-4000-8000-00000000001a', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000004', 'SENT', 'CHANGE_REQUESTED', NULL, now() - interval '1 day'),

  ('50000000-0000-4000-8000-00000000001b', 'a0000000-0000-4000-8000-000000000001', '20000000-0000-4000-8000-000000000005', NULL, 'DRAFT', 'c0000000-0000-4000-8000-000000000001', now() - interval '40 minutes')
ON CONFLICT (id) DO NOTHING;

-- El envío cuelga de la versión, no de la cotización: el link se emite por envío y por canal.
INSERT INTO quote_send (id, account_id, version_id, channel_id, public_token, format, sent_at, expires_at, tracking_status)
SELECT v.id, v.account_id, v.version_id, v.channel_id, v.public_token, v.format, v.sent_at, v.expires_at, v.tracking_status
FROM (VALUES
  ('60000000-0000-4000-8000-000000000001'::uuid, 'a0000000-0000-4000-8000-000000000001'::uuid, '30000000-0000-4000-8000-000000000003'::uuid,
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000001' AND type = 'WEBAPP' AND identifier IS NULL),
   'seed-token-sent-0000000000000001', 'WEBAPP_LINK'::send_format, now() - interval '5 days' + interval '1 hour', now() + interval '2 days', 'VIEWED'::send_tracking_status),
  ('60000000-0000-4000-8000-000000000002'::uuid, 'a0000000-0000-4000-8000-000000000001'::uuid, '30000000-0000-4000-8000-000000000004'::uuid,
   (SELECT id FROM channel WHERE branch_id = 'b0000000-0000-4000-8000-000000000001' AND type = 'WHATSAPP' AND identifier IS NULL),
   'seed-token-change-000000000002', 'MESSAGE'::send_format, now() - interval '7 days', now() - interval '1 day', 'DELIVERED'::send_tracking_status)
) AS v(id, account_id, version_id, channel_id, public_token, format, sent_at, expires_at, tracking_status)
ON CONFLICT (id) DO NOTHING;
