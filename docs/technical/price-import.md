# Importación de precios

El backoffice permite actualizar precios por archivo desde
`/settings/prices`. La operación siempre está acotada a una sucursal y requiere
un usuario con rol `ADMIN`.

## Flujo

1. El administrador selecciona la sucursal y puede descargar sus precios
   vigentes con **Exportar precios**. `GET /v1/product-prices/export` genera un
   `.xlsx` con una hoja `Precios` precargada y una hoja `Instrucciones`.
2. El administrador edita el archivo exportado —o prepara un `.xlsx` o `.csv`
   compatible— y lo carga en la misma pantalla.
3. `POST /v1/product-prices/import/preview` lee el archivo y devuelve todas las
   filas con el producto encontrado, el precio vigente, el valor propuesto y los
   errores de validación. Esta operación no escribe en la base.
4. La pantalla permite confirmar únicamente cuando todas las filas son válidas.
5. `POST /v1/product-prices/import/confirm` vuelve a validar el contenido y, en
   una única transacción, cierra la vigencia del precio actual e inserta las
   nuevas filas de `product_price`.

No se modifican cotizaciones existentes. Cada `quote_item` conserva los
snapshots de precio y precio mínimo tomados al armar su versión; re-preciar una
cotización es una acción distinta.

## Formato del archivo

La primera hoja de un `.xlsx`, o el contenido de un `.csv`, debe tener estas
columnas:

| Columna         | Obligatoria | Descripción                                     |
| --------------- | ----------- | ----------------------------------------------- |
| `codigo`        | sí          | Código único del producto dentro de la cuenta.  |
| `precio`        | sí          | Precio de venta mayor a cero, con 2 decimales.  |
| `precio_minimo` | no          | Piso del motor de descuentos; no supera precio. |
| `moneda`        | no          | Código ISO de 3 letras; por defecto `ARS`.      |
| `condiciones`   | no          | Condición textual, hasta 255 caracteres.        |

El archivo exportado agrega la columna informativa `producto`. La importación
la ignora y vincula cada fila exclusivamente por `codigo`. La exportación
incluye solo productos activos que ya tienen un precio vigente en la sucursal;
para no actualizar un producto basta con eliminar su fila antes de importar.

También se aceptan los encabezados equivalentes en inglés: `code`, `price`,
`min_price`, `currency` y `conditions`. Los CSV pueden usar coma o punto y coma
como separador.

El archivo completo se rechaza para confirmación si contiene códigos repetidos,
productos inexistentes o inactivos, importes inválidos, un precio mínimo mayor
al de venta, una moneda inválida o condiciones demasiado largas.

## Configuración

`PRICE_IMPORT_MAX_BYTES` define el tamaño máximo del archivo en bytes. El valor
por defecto es `5242880` (5 MiB).

## Endpoints auxiliares

`GET /v1/branches` devuelve las sucursales activas accesibles para el usuario y
alimenta el selector del backoffice. La sucursal elegida viaja en
`X-Branch-Id`, donde el middleware vuelve a validar acceso antes de ejecutar la
importación.
