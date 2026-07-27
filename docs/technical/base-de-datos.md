# Base de datos

PostgreSQL 16 + pgvector. El modelo son 35 tablas con PK UUID v4, enums nativos y
plata en `NUMERIC(14,2)`.

## Qué es fuente y qué es referencia

| Archivo                                  | Rol                                                      |
| ---------------------------------------- | -------------------------------------------------------- |
| `apps/api/migrations/*.sql`              | **Fuente ejecutable.** Lo único que escribe en una base. |
| `apps/api/database/01_create_tables.sql` | Referencia consolidada. Se lee, no se aplica.            |
| `apps/api/database/02_seed_dev.sql`      | Datos de desarrollo. Idempotente.                        |

Un cambio de schema ships una migración goose **y** actualiza la referencia en el
mismo PR. La referencia es lo que se lee para escribir la lista de columnas de un
`SELECT`, el orden del scan y los campos del struct de dominio.

Mientras la referencia sea el `Up` de la cadena, esto tiene que salir vacío:

```bash
diff -B \
  <(sed -n '/CREATE EXTENSION/,$p' apps/api/migrations/00001_init_schema.sql \
    | sed '/-- +goose Down/,$d' | grep -v '^-- +goose') \
  <(sed -n '/CREATE EXTENSION/,$p' apps/api/database/01_create_tables.sql)
```

Con más de una migración la comparación deja de ser textual: se regenera la
referencia contra una base migrada.

## Levantar una base

```bash
pnpm db:init      # postgres + goose up + seed
pnpm db:migrate   # aplicar migraciones pendientes
pnpm db:seed      # solo el seed (idempotente)
pnpm db:reset     # borrar el volumen y reconstruir
pnpm db:create-migration <nombre>
```

`POSTGRES_PORT` (default 5432) cambia el puerto que publica el contenedor cuando
otro Postgres local ya lo tiene tomado. Tiene que quedar en sync con las URLs.

## Dos roles de conexión

| Variable             | Rol                                     | Para qué                                                                        |
| -------------------- | --------------------------------------- | ------------------------------------------------------------------------------- |
| `DATABASE_URL`       | `coti_app` — restringido, `NOBYPASSRLS` | Toda query de request.                                                          |
| `DATABASE_ADMIN_URL` | owner                                   | Migraciones, cron de follow-up, y los lookups que legítimamente cruzan cuentas. |

**Nunca usar el rol owner para una query de request.** Bypassea RLS.

Los tres casos legítimos del owner:

1. **Migraciones** — crean tablas y otorgan permisos.
2. **Cron de follow-up** — barre cotizaciones de todas las cuentas.
3. **Lookups pre-auth** — login por email (todavía no se sabe la cuenta) y
   resolución de `quote_send.public_token` desde la webapp sin sesión. El patrón
   correcto para el token: el owner resuelve token → `account_id`, y el resto del
   request sigue con el rol restringido y la GUC seteada.

## Aislamiento por cuenta (RLS)

Toda tabla con dueño de tenant lleva `account_id` —incluidas las hijas— y una
política que compara contra `app_current_account_id()`, que lee la GUC
`app.current_account_id`.

```sql
BEGIN;
  SET LOCAL app.current_account_id = '<uuid de la cuenta>';
  -- queries del request
COMMIT;
```

Eso es para psql. **Desde Go la GUC la setea `repository.DB.InTenantTx`** con
`SELECT set_config('app.current_account_id', $1, true)`, y es el único camino para una
query de request. No se usa `SET LOCAL` en código: esa forma no acepta bind
parameters, así que obligaría a interpolar un valor del request dentro del SQL.

**Va en cada transacción**: el pool reutiliza conexiones, así que no se hereda. Y
**toda query de request tiene que ir dentro de una transacción**, lecturas incluidas:
la GUC es transaction-scoped, así que una query sobre el pool pelado corre fuera del
scope, no matchea ninguna política y lee cero filas en silencio.

Los tres casos que legítimamente cruzan cuentas usan `db.CrossAccount()` (o
`db.AdminTx()` para escrituras multi-paso), que van por el pool del owner.

Se enforcea la **cuenta**, no la sucursal: un admin lee legítimamente todas las
sucursales de su cuenta, así que el scoping por `branch_id` queda en la
aplicación. RLS es la segunda red, no un reemplazo del `WHERE`: los predicados
explícitos siguen yendo en cada query para que el plan use los índices.

Dos trampas conocidas:

- **Anda en psql, vacío en la app.** psql como owner bypassea RLS; la app no.
  Para reproducir lo que ve la app, conectarse como `coti_app` y setear la GUC.
- **Una migración que agrega una tabla y se olvida el GRANT** rompe la app en
  runtime, no en la migración. `ALTER DEFAULT PRIVILEGES` cubre las tablas nuevas
  creadas por el owner; si una migración crea algo por otra vía, hay que otorgar a
  mano.

## Invariantes en la base

Lo que se puede expresar en el schema, se expresa en el schema:

| Índice                        | Invariante                                          |
| ----------------------------- | --------------------------------------------------- |
| `uq_quote_rfq`                | 1-a-1 rfq→quote                                     |
| `uq_quote_version_draft`      | un solo draft en curso por cotización               |
| `uq_message_batch_open`       | una sola ventana de mensajes abierta por cotización |
| `uq_message_batch_processing` | un solo batch procesando (cola FIFO)                |
| `uq_quote_send_public_token`  | el token del magic link es único                    |

## Catálogo

El catálogo es **de la cuenta**: `product`, `product_synonym` y
`product_alternative` cuelgan de `account_id`. Un producto es una fila por cuenta,
con un solo embedding y un solo juego de sinónimos.

Lo que varía por sucursal:

- `branch_product` — si la sucursal lo tiene, con qué stock.
- `product_price` — precio y `min_price` por sucursal, versionado por vigencia.

La búsqueda semántica filtra por `account_id` y joinea `branch_product` para
excluir lo que la sucursal no vende. Como el índice ANN filtra **después** de
ordenar, hay que sobre-pedir (`LIMIT k × factor`) y recortar en el service, o el
filtro de sucursal puede dejar menos de K resultados.

## Índice vectorial

`idx_product_embedding` (ivfflat, `vector_cosine_ops`) va **comentado** en la
migración: con la tabla vacía queda subóptimo. Se crea después de cargar el
catálogo y generar los embeddings.

## Enums

Nativos de PostgreSQL, valores en **inglés UPPERCASE**. Los labels en español
viven en el i18n del frontend; nunca se guardan en la base. Agregar un valor
requiere `ALTER TYPE ... ADD VALUE` — aceptable porque los enums del dominio son
cerrados por diseño.

El ciclo de vida está repartido entre dos entidades: `rfq.status`
(`RECEIVED`, `GENERATED`) y `quote.current_status` (`DRAFT` en adelante). La
cotización nace en la transición RECEIVED → GENERATED con `current_status = DRAFT`
(materiales listos, precios no aceptados), porque los ítems extraídos solo pueden
vivir bajo una versión de cotización.

## `created_at` / `updated_at`

`created_at` en todas. `updated_at` + trigger `set_updated_at()` solo en las que
mutan in-place: `account`, `branch`, `app_user`, `product`, `branch_product`,
`combo`, `client`, `channel`, `rfq`, `quote`, `promotion`. Las append-only no lo
llevan, y eso comunica en el propio schema qué tabla es log y qué tabla es estado
vivo.
