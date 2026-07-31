# Catálogo

El catálogo es **de la cuenta**: `product`, `product_synonym` y `product_alternative`
cuelgan de `account_id`. Un producto es una fila por cuenta, con un solo embedding y un
solo juego de sinónimos y alternativas. Lo que varía por sucursal —disponibilidad, stock
y precio— vive en `branch_product` y `product_price`, y tiene sus propias rutas.

Por eso ninguna ruta de esta página necesita `X-Branch-Id`: son todas de nivel cuenta.

## Endpoints

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

Todas piden sesión (`RequireTenant`). Los montos y las cantidades no aparecen acá: el
producto no lleva precio.

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

Los handlers llevan las anotaciones swaggo (`@Summary`, `@Param`, `@Success`, `@Router`), y
los errores tienen una forma única, `dto.ErrorResponse`, para que el spec describa un solo
cuerpo de error.

**Falta el toolchain que genera y sirve el spec** (el CLI de `swag`, el paquete generado y la
ruta que lo publica), más las anotaciones de los handlers de autenticación, que son
anteriores a esta convención. Es su propio ticket: mete un paso de codegen y artefactos
generados al repo, y ninguna de las dos cosas se decide de costado.
