-- +goose Up

CREATE TABLE product_family (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name       VARCHAR(255) NOT NULL UNIQUE,
  sort_order INTEGER NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE product_subgroup (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id  UUID NOT NULL REFERENCES product_family(id) ON DELETE RESTRICT,
  name       VARCHAR(255) NOT NULL,
  sort_order INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_product_subgroup_family_name UNIQUE (family_id, name),
  CONSTRAINT uq_product_subgroup_family_order UNIQUE (family_id, sort_order),
  CONSTRAINT uq_product_subgroup_id_family UNIQUE (id, family_id)
);

INSERT INTO product_family (name, sort_order) VALUES
  ('MATERIALES DE CONSTRUCCION', 1), ('CERAMICAS Y PORCELANATOS', 2),
  ('CHAPAS Y PERFILES', 3), ('ABERTURAS', 4), ('TANQUES', 5),
  ('AGUA Y CLOACAS', 6), ('GAS', 7), ('BAÑO', 8), ('COCINA', 9),
  ('DURLOCK', 10), ('FERRETERIA', 11), ('REFRACTARIOS', 12), ('DECO', 13);

INSERT INTO product_subgroup (family_id, name, sort_order)
SELECT f.id, v.name, v.sort_order
FROM (VALUES
  ('MATERIALES DE CONSTRUCCION', 'BOLSAS', 1),
  ('MATERIALES DE CONSTRUCCION', 'HIERROS', 2),
  ('MATERIALES DE CONSTRUCCION', 'MALLAS', 3),
  ('MATERIALES DE CONSTRUCCION', 'LADRILLOS', 4),
  ('MATERIALES DE CONSTRUCCION', 'ADITIVOS', 5),
  ('MATERIALES DE CONSTRUCCION', 'ARIDOS', 6),
  ('MATERIALES DE CONSTRUCCION', 'HORMIGON', 7),
  ('MATERIALES DE CONSTRUCCION', 'TECHO', 8),
  ('MATERIALES DE CONSTRUCCION', 'PINTURAS ASFALTICAS', 9),
  ('CERAMICAS Y PORCELANATOS', 'PASTINAS Y PEGAMENTOS', 1),
  ('CERAMICAS Y PORCELANATOS', 'CERAMICAS', 2),
  ('CERAMICAS Y PORCELANATOS', 'PORCELANATOS', 3),
  ('CERAMICAS Y PORCELANATOS', 'ACCESORIOS PARA CERAMICAS', 4),
  ('CERAMICAS Y PORCELANATOS', 'GUARDAS', 5),
  ('CHAPAS Y PERFILES', 'PERFILES', 1), ('CHAPAS Y PERFILES', 'CHAPAS', 2),
  ('CHAPAS Y PERFILES', 'ESTRUCTURALES', 3),
  ('CHAPAS Y PERFILES', 'COMPLEMENTOS PARA CHAPAS', 4),
  ('ABERTURAS', 'VENTANAS', 1), ('ABERTURAS', 'PUERTAS DE INTERIOR', 2),
  ('ABERTURAS', 'PUERTAS DE EXTERIOR', 3),
  ('AGUA Y CLOACAS', 'CLOACAS', 1), ('AGUA Y CLOACAS', 'AGUA', 2),
  ('GAS', 'GABINETES', 1), ('GAS', 'TUBOS Y ACCESORIOS', 2),
  ('BAÑO', 'ARTEFACTOS PARA BAÑO', 1), ('BAÑO', 'VANITORYS', 2),
  ('BAÑO', 'GRIFERIA PARA BAÑO', 3), ('BAÑO', 'ACCESORIOS PARA BAÑO', 4),
  ('BAÑO', 'BOTIQUINES', 5),
  ('COCINA', 'MESADAS', 1), ('COCINA', 'BACHAS DE COCINA', 2),
  ('COCINA', 'BAJO MESADA', 3), ('COCINA', 'GRIFERIA PARA COCINA', 4),
  ('COCINA', 'ACCESORIOS PARA COCINA', 5),
  ('DURLOCK', 'PLACAS DE DURLOCK', 1), ('DURLOCK', 'PERFILES PARA DURLOCK', 2),
  ('DURLOCK', 'COMPLEMENTARIOS PARA DURLOCK', 3),
  ('FERRETERIA', 'PLOMERIA', 1), ('FERRETERIA', 'HERRAMIENTAS DE CORTE', 2),
  ('FERRETERIA', 'HERRAMIENTAS DE MANO', 3), ('FERRETERIA', 'PALAS', 4),
  ('FERRETERIA', 'ALBAÑILERIA', 5), ('FERRETERIA', 'REJILLAS', 6),
  ('FERRETERIA', 'VARIOS DE FERRETERIA', 7), ('FERRETERIA', 'ZINGUERIA', 8),
  ('FERRETERIA', 'CANALETAS', 9),
  ('DECO', 'MICA', 1), ('DECO', 'CIELO RASO', 2), ('DECO', 'ZOCALOS', 3),
  ('DECO', 'PERSIANAS DE PVC', 4), ('DECO', 'MOLDURAS', 5)
) AS v(family_name, name, sort_order)
JOIN product_family f ON f.name = v.family_name;

ALTER TABLE product
  ADD COLUMN family_id UUID REFERENCES product_family(id) ON DELETE RESTRICT,
  ADD COLUMN subgroup_id UUID,
  ADD CONSTRAINT fk_product_subgroup_family FOREIGN KEY (subgroup_id, family_id)
    REFERENCES product_subgroup(id, family_id) ON DELETE RESTRICT;

ALTER TABLE product DROP COLUMN category;
ALTER TABLE product_price DROP COLUMN conditions;

ALTER TABLE promotion_condition_item DROP CONSTRAINT ck_pci_target;
ALTER TABLE promotion_condition_item
  ADD COLUMN family_id UUID REFERENCES product_family(id) ON DELETE RESTRICT,
  ADD COLUMN subgroup_id UUID REFERENCES product_subgroup(id) ON DELETE RESTRICT,
  DROP COLUMN category,
  ADD CONSTRAINT ck_pci_target CHECK (
    product_id IS NOT NULL OR family_id IS NOT NULL OR subgroup_id IS NOT NULL
  );

CREATE INDEX idx_product_family_id ON product(family_id);
CREATE INDEX idx_product_subgroup_id ON product(subgroup_id);
CREATE INDEX idx_product_subgroup_family_id ON product_subgroup(family_id);

REVOKE INSERT, UPDATE, DELETE ON product_family, product_subgroup FROM coti_app;
GRANT SELECT ON product_family, product_subgroup TO coti_app;

-- +goose Down

ALTER TABLE promotion_condition_item DROP CONSTRAINT ck_pci_target;
ALTER TABLE promotion_condition_item
  ADD COLUMN category VARCHAR(255),
  DROP COLUMN subgroup_id,
  DROP COLUMN family_id,
  ADD CONSTRAINT ck_pci_target CHECK (product_id IS NOT NULL OR category IS NOT NULL);

ALTER TABLE product_price ADD COLUMN conditions VARCHAR(255);
ALTER TABLE product ADD COLUMN category VARCHAR(255);
ALTER TABLE product DROP COLUMN subgroup_id;
ALTER TABLE product DROP COLUMN family_id;

DROP TABLE product_subgroup;
DROP TABLE product_family;
