# Catálogo

El catálogo es **de la cuenta**: `product`, `product_synonym` y `product_alternative`
cuelgan de `account_id`. Un producto es una fila por cuenta, con un solo embedding y un
solo juego de sinónimos y alternativas. Lo que varía por sucursal —disponibilidad, stock
y precio— vive en `branch_product` y `product_price`.

Ese corte parte las rutas en dos: las de nivel cuenta no miran `X-Branch-Id`, y las de
sucursal lo necesitan para escribir.

## Endpoints

De nivel cuenta:

| Método   | Ruta                                                    | Qué hace                                       |
| -------- | ------------------------------------------------------- | ---------------------------------------------- |
| `GET`    | `/v1/products`                                          | Lista paginada, con búsqueda y filtro de rubro |
| `POST`   | `/v1/products`                                          | Alta                                           |
| `GET`    | `/v1/products/{productId}`                              | Uno                                            |
| `PUT`    | `/v1/products/{productId}`                              | Modificación                                   |
| `DELETE` | `/v1/products/{productId}`                              | Baja lógica                                    |
| `GET`    | `/v1/products/{productId}/synonyms`                     | Sinónimos del producto                         |
| `POST`   | `/v1/products/{productId}/synonyms`                     | Agregar sinónimo                               |
| `DELETE` | `/v1/products/{productId}/synonyms/{synonymId}`         | Quitar sinónimo                                |
| `GET`    | `/v1/products/{productId}/alternatives`                 | Alternativas, en la dirección pedida           |
| `POST`   | `/v1/products/{productId}/alternatives`                 | Definir alternativa                            |
| `DELETE` | `/v1/products/{productId}/alternatives/{alternativeId}` | Quitar alternativa                             |

Por sucursal:

| Método | Ruta                                    | Qué hace                       |
| ------ | --------------------------------------- | ------------------------------ |
| `GET`  | `/v1/products/{productId}/availability` | Dónde se vende y con qué stock |
| `PUT`  | `/v1/products/{productId}/availability` | Definir disponibilidad y stock |
| `GET`  | `/v1/products/{productId}/prices`       | Historial de vigencias         |
| `POST` | `/v1/products/{productId}/prices`       | Poner un precio en vigencia    |

Todas piden sesión (`RequireTenant`).

## Código único por cuenta, y por qué el vacío es NULL

`uq_product_account_code` es un índice único **parcial** sobre `(account_id, code)` donde
`code IS NOT NULL`. Un código repetido dentro de la cuenta devuelve **409**; la misma
cadena en otra cuenta es válida.

El service normaliza: recorta espacios y **convierte el vacío en NULL**. Es necesario, no
cosmético — dos productos con código `''` chocarían contra el índice, mientras que dos con
`NULL` son exactamente lo que parece un catálogo sin numerar.

## Modificación y baja lógica

`PUT` **reemplaza** los atributos editables: un campo nullable omitido queda en NULL, así
que el cliente manda el producto como tiene que quedar. `is_active` es la excepción —
omitido no se toca, para que un formulario de edición no reviva sin querer un producto dado
de baja. Reactivar es explícito: `"is_active": true`.

`DELETE` es **baja lógica** (`is_active = FALSE`). La fila sobrevive porque los ítems de
cotizaciones cerradas y el historial de precios la referencian; borrarla reescribiría
historia. Repetir la llamada no falla. La lista esconde lo inactivo salvo que venga
`include_inactive=true`.

## Sinónimos

Términos de oficio que mejoran el matching léxico (mitigación de R06). `source` dice de
dónde salió el término: `MANUAL` el que cargó una persona, `LEARNED` el que propuso el
pipeline de matching. Si no viene, es `MANUAL`.

La columna es `VARCHAR(64)` en el schema, no un enum nativo: el conjunto cerrado que la API
acepta vive en `domain.SynonymSource`. Las filas del seed traen `seed`, que se lee sin
problema y no lo escribe ningún endpoint.

Un término repetido en el mismo producto devuelve **409**, comparando sin distinguir
mayúsculas: "Portland" y "portland" son el mismo término para un matcher. El chequeo va como
`NOT EXISTS` dentro del `INSERT` porque el schema no tiene un índice único sobre
`(product_id, term)` que sirva de `ON CONFLICT`.

## Alternativas

`product_alternative` liga un producto base con otro que puede reemplazarlo, tipado como
`EQUIVALENT`, `PREMIUM` o `ECONOMY`. `uq_product_alternative` permite un solo link por par
ordenado: repetirlo devuelve **409**, y para cambiarle el tipo se borra y se vuelve a crear.

**La dirección es un parámetro, no dos implementaciones.** `direction` elige de qué punta se
lee la relación:

- `OUTGOING` (default) — qué se puede ofrecer en lugar de este producto. Lo que pide el motor
  de recomendaciones.
- `INCOMING` — de qué productos este es la alternativa. Lo que pide la intención de mejorar
  calidad.

Un producto no puede ser su propia alternativa (**422**). El link se borra desde cualquiera
de sus dos puntas, y el `productId` de la ruta tiene que ser una de las dos: un link entre
otros dos productos no se borra a través de un tercero.

## El chequeo que la base no hace

Antes de escribir un sinónimo o una alternativa, el service **lee el producto dentro del
scope del tenant**. No es redundante:

> Los chequeos de integridad referencial —FKs, unicidad— **bypassean row level security**.

Así que una FK a `product` acepta sin chistar el id de un producto de otra cuenta, y quedaría
un sinónimo con `account_id` propio colgado de un producto invisible. La política solo mira
la columna `account_id` de la fila que se inserta, que es la nuestra. El `SELECT` previo es
lo único que cierra ese agujero, y hay tests que lo fijan (uno de ellos afirma que la FK
efectivamente lo permite, para que se caiga si algún día eso cambia).

## Disponibilidad por sucursal

`branch_product` dice si la sucursal vende el producto y con cuánto stock. `PUT` hace
upsert contra `uq_branch_product`, la unicidad `(branch_id, product_id)` del schema, así que
quien llama no tiene que saber si es la primera vez.

`stock` es un string decimal, y **ausente no es cero**: NULL significa que la sucursal no
lleva stock de ese ítem, cero significa que no le queda. `is_active` sin especificar es
`true`, porque el sentido de la llamada normalmente es que sí lo vende; ponerlo en `false` es
cómo una sucursal deja de ofrecer algo que la cuenta sigue catalogando.

## Precios por sucursal, versionados por vigencia

**Un precio nunca se sobrescribe.** Poner un precio abre una vigencia nueva y cierra la
anterior en el mismo instante: `valid_to` de la vieja queda igual al `valid_from` de la
nueva, y las dos escrituras van en **una transacción**, así que una caída en el medio no
puede dejar el producto con dos vigencias abiertas ni con ninguna.

Es lo que mantiene explicable una cotización congelada el mes pasado: el precio que aplicaba
en ese momento sigue en la tabla.

- `valid_from` sin especificar es ahora.
- `valid_from` **anterior** al inicio de la vigencia abierta se rechaza (**422**): cerraría
  un período antes de empezarlo, y reescribiría qué precio aplicaba en un momento ya
  cotizado.
- El primer precio de un producto en una sucursal no cierra nada.
- `currency` sin especificar es `ARS`.
- `min_price` es el **piso del motor de descuentos**, así que no puede superar al precio que
  pisa (**422**).
- `GET` devuelve el historial completo, vigencias cerradas incluidas, de la más nueva a la
  más vieja.

Ambas escrituras necesitan sucursal activa: sin `X-Branch-Id` no hay destino correcto, y
adivinar uno le pondría precio a la sucursal equivocada (**422**, no un default silencioso).
Las lecturas sí funcionan sin el header, y ahí devuelven todas las sucursales de la cuenta —
que es cómo un admin las compara.

## La plata viaja como string decimal

`price`, `min_price` y `stock` son `NUMERIC(14,2)` en la base, `decimal.Decimal` en Go y
**strings decimales** en el JSON. Nunca floats: un número JSON perdería precisión en el ida y
vuelta. El codec de pgx se registra por conexión en `AfterConnect`, porque el pool abre
conexiones cuando quiere, incluidos los reemplazos de las que se mueren.

El service rechaza lo que la columna no puede guardar exacto:

- más de dos decimales — Postgres redondearía el tercero sin avisar, y en plata eso es un
  defecto, no una preferencia de redondeo;
- más de 12 dígitos enteros — un dígito de más tipeado tiene que ser un mensaje accionable y
  no un 500;
- negativos.

## Paginación

`limit` y `offset`. Sin `limit`, `CATALOG_DEFAULT_PAGE_SIZE`; por encima del techo,
`CATALOG_MAX_PAGE_SIZE`. `total` cuenta **todas** las filas que matchean el filtro, no las de
la página, y sale de un `count(*) OVER ()` en la misma query: un solo round trip, y el total
no puede contradecir a la página que describe.

## Configuración

| Variable                    | Default | Para qué                                   |
| --------------------------- | ------- | ------------------------------------------ |
| `CATALOG_DEFAULT_PAGE_SIZE` | 50      | Tamaño de página cuando no viene `limit`   |
| `CATALOG_MAX_PAGE_SIZE`     | 200     | Techo de `limit`, para que nadie pida todo |

## Especificación de la API

Los quince handlers están anotados y aparecen en el spec generado. Cómo se genera, se sirve y
se verifica: [especificacion-api.md](especificacion-api.md).
