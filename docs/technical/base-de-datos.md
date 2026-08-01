# Base de datos

PostgreSQL 16 + pgvector. El modelo son 36 tablas con PK UUID v4, enums nativos y
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

Con más de una migración la comparación deja de ser textual —la referencia describe el
resultado, no la secuencia que llega a él— así que se compara el schema que produce cada
uno. Dos bases vacías, una migrada y otra con la referencia aplicada, y un `pg_dump` de
cada una: la diferencia tiene que ser vacía salvo los tokens de sesión que `pg_dump`
genera al azar.

```bash
docker exec migrada   pg_dump -U coti -d coti --schema-only --no-owner --no-privileges \
  --exclude-table=goose_db_version > /tmp/desde-migraciones.sql
docker exec referencia pg_dump -U coti -d coti --schema-only --no-owner --no-privileges \
  > /tmp/desde-referencia.sql
diff /tmp/desde-referencia.sql /tmp/desde-migraciones.sql
```

Esto también atrapa el orden de columnas, que es fácil de perder de vista: `ALTER TABLE
ADD COLUMN` agrega al final, así que una columna nueva va última en la tabla real aunque
quede más linda al lado de sus hermanas. La referencia sigue al schema físico, no al
gusto.

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

**Un `ON CONFLICT DO NOTHING` no es idempotente por sí solo:** necesita una restricción
única con la cual chocar. Sobre una tabla cuyo único índice único es la PK —un uuid
aleatorio— no filtra nada y cada corrida vuelve a insertar todo. Es lo que le pasaba a
`product_synonym` hasta `uq_product_synonym_term`. Al sembrar una tabla, el destino del
`ON CONFLICT` va explícito, y si no existe la clave natural que lo sostenga, falta un índice.

**El seed es idempotente, no convergente.** Inserta con `ON CONFLICT ... DO NOTHING`, así
que crea lo que falta pero **nunca reescribe una fila que ya existe**. Si cambia un valor del
seed —un estado, un total, un nombre— la base que ya lo tenía se queda con el viejo, y
`pnpm db:seed` no lo corrige. Para tomar cambios de valores va `pnpm db:reset`, que reconstruye
por la cadena entera. Es deliberado: un seed que sobreescribe te borra los datos con los que
estabas probando.

Dos casos donde `db:reset` es la única salida:

- **Cambió un valor del seed** (lo de arriba).
- **Se editó una migración ya aplicada.** El `down` describe el `up` nuevo, así que contra una
  base que corrió el viejo intenta revertir cosas que nunca creó y falla. Pasa mientras un PR
  de migración está en revisión: quien ya la corrió tiene que resetear al bajar cambios. La
  salida **no** es llenar el `down` de `IF EXISTS` para tolerar estados a medias.

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
- **Una migración que agrega una tabla y se olvida la política de RLS no falla:
  devuelve las filas de todas las cuentas.** Una tabla nueva nace con RLS deshabilitada,
  así que el rol de la app la lee entera sin error, sin permiso denegado y sin nada que
  lo delate — exposición cross-account en silencio. Toda tabla nueva con `account_id`
  ship su `ENABLE ROW LEVEL SECURITY` y su política en la misma migración.
- **El GRANT, en cambio, ya está cubierto:** `ALTER DEFAULT PRIVILEGES` alcanza a las
  tablas que crea el owner, y las migraciones corren con ese rol. Se escribe explícito
  igual, para no depender de que el rol que migra sea siempre el mismo.

## Invariantes en la base

Lo que se puede expresar en el schema, se expresa en el schema:

| Índice                                   | Invariante                                          |
| ---------------------------------------- | --------------------------------------------------- |
| `uq_quote_rfq` + `quote.rfq_id NOT NULL` | 1-a-1 rfq→quote                                     |
| `uq_quote_version_draft`                 | un solo draft en curso por cotización               |
| `uq_message_batch_open`                  | una sola ventana de mensajes abierta por cotización |
| `uq_message_batch_processing`            | un solo batch procesando (cola FIFO)                |
| `uq_quote_send_public_token`             | el token del magic link es único                    |
| `uq_product_account_code`                | el código de producto es único dentro de la cuenta  |
| `uq_product_synonym_term`                | un término por producto, sin distinguir mayúsculas  |
| `uq_channel_branch_type_no_identifier`   | un solo canal sin identificador por sucursal y tipo |

**Una restricción de unicidad no compara NULLs**, así que sobre una columna nullable deja
escapar todas las filas vacías. Por eso el 1-a-1 necesita el NOT NULL además del índice:
`uq_channel_branch_type_identifier` sola no limita los canales sin identificador, y ahí
entra el índice parcial. Al blindar una invariante sobre una columna nullable, la única
salida es el NOT NULL o un índice parcial sobre el caso NULL.

## Catálogo

El catálogo es **de la cuenta**: `product`, `product_synonym`, `product_alternative` y
`combo` cuelgan de `account_id`. Un producto es una fila por cuenta, con un solo
embedding y un solo juego de sinónimos.

Lo que varía por sucursal:

- `branch_product` — si la sucursal lo tiene, con qué stock.
- `product_price` — precio y `min_price` por sucursal, versionado por vigencia.
- `branch_combo` — si la sucursal ofrece el combo. Sin precio y sin stock: los dos se
  derivan de los items, que ya están valorizados por sucursal.

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

Tres cosas que muerden al migrar un enum:

- **Un valor recién agregado no se puede usar en la misma transacción que lo agregó**
  (`unsafe use of new value`). goose envuelve cada archivo en una transacción, así que
  una migración que agrega el valor **y** escribe filas con él necesita partirse en dos.
- **Renombrar sí se puede** (`ALTER TYPE ... RENAME VALUE`): es solo metadata, las filas
  existentes siguen apuntando a la misma entrada y el valor queda usable en el acto.
  Cuando el valor viejo ya significaba lo nuevo, renombrar le gana a agregar.
- **Quitar un valor no existe.** Hay que recrear el tipo y castear cada columna que lo
  usa, así que conviene saber cuáles son antes de empezar.

El ciclo de vida está repartido entre dos entidades: `rfq.status`
(`RECEIVED`, `GENERATED`) y `quote.current_status` (`DRAFT` en adelante). La
cotización nace en la transición RECEIVED → GENERATED con `current_status = DRAFT`
(materiales listos, precios no aceptados), porque los ítems extraídos solo pueden
vivir bajo una versión de cotización.

## `created_at` / `updated_at`

`created_at` en todas. `updated_at` + trigger `set_updated_at()` solo en las que
mutan in-place: `account`, `branch`, `app_user`, `product`, `branch_product`,
`combo`, `branch_combo`, `client`, `channel`, `rfq`, `quote`, `promotion`. Las
append-only no lo llevan, y eso comunica en el propio schema qué tabla es log y qué
tabla es estado vivo.

Las tablas de proceso, en cambio, llevan **un timestamp por transición que importa** en
vez de un `updated_at` genérico: `notification.sent_at`, `rfq_attachment.processed_at`,
`quote.followup_flagged_at`, `quote.archived_at`. Un `updated_at` dice que algo cambió;
estos dicen qué cambió y cuándo, que es lo que después se consulta.
