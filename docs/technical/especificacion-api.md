# Especificación de la API

El contrato se **genera desde las anotaciones de los handlers** con
[swaggo/swag](https://github.com/swaggo/swag). Vive al lado del código que lo implementa, así
que se desactualiza menos que un YAML escrito a mano.

```bash
pnpm docs:api          # regenerar apps/api/docs/
pnpm docs:api:check    # regenerar y fallar si el resultado difiere de lo commiteado
```

Con la API levantada, la UI queda en **http://localhost:8000/swagger/index.html** y el spec
crudo en `/swagger/doc.json`.

## Lo generado se commitea, y CI compara

`apps/api/docs/` (`docs.go`, `swagger.json`, `swagger.yaml`) está en git. El paso
**"OpenAPI spec is up to date"** de `ci.api.yml` regenera y corre `git diff --exit-code`: si
alguien toca un handler y no regenera, CI se cae.

Ese paso es el punto del asunto. **Una anotación que nadie compila es un comentario**, y se
despega del código sin que nada avise. El diff es lo que las convierte en algo verificado.

Dos consecuencias que hay que respetar:

- **Prettier no toca `apps/api/docs/`** (está en `.prettierignore`). Formatearlo haría que lo
  commiteado difiera de lo que emite swag, y CI compara exactamente esas dos cosas.
- **El CLI está pineado en `go.mod`** con la directiva `tool` de Go 1.24+
  (`go tool swag`), no con un `@latest` flotante: dos personas con versiones distintas
  generan specs distintos y el diff se vuelve ruido.

## Por qué swag v1 y no v2

v2 emite OpenAPI 3.1, que es lo que uno querría. Pero al día de hoy está en
**`v2.0.0-rc5`** —release candidate— y `gin-swagger/v2` **no tiene ninguna versión
publicada**, así que no hay forma estable de servirlo. Pinear el generador del contrato de la
API a un RC sin acompañante estable no vale el 3.1.

v1 emite Swagger 2.0, que lee cualquier herramienta, y la sintaxis de anotaciones es
prácticamente la misma, así que migrar cuando v2 estabilice es barato. Revisar en ese momento.

## El spec no se publica en producción

La ruta `/swagger/*any` se monta solo cuando `ENV != production`. Describe una API interna:
publicarla le daría a un lector sin credenciales toda la superficie a cambio de nada. Los
consumidores son las dos apps web de este repo y quien las escribe.

## Anotar un handler

El bloque va en el doc comment, indentado con tabs (así `gofmt` lo trata como bloque
preformateado y no lo reacomoda):

```go
// Get returns one catalog item.
//
//	@Summary	Get a product
//	@Tags		catalog
//	@Produce	json
//	@Security	BearerAuth
//	@Param		productId	path		string	true	"Product id"
//	@Success	200			{object}	dto.ProductResponse
//	@Failure	404			{object}	dto.ErrorResponse
//	@Router		/v1/products/{productId} [get]
```

Reglas que ya están fijadas:

- **`@Router` lleva la ruta completa**, con `/v1` incluido. `basePath` es `/` porque las sondas
  (`/health`, `/ready`) viven fuera de `/v1` y un basePath de `/v1` las documentaría mal.
- **`@Security BearerAuth`** en toda ruta que necesite sesión; las de `/v1/public` no lo llevan.
- **Los errores son siempre `dto.ErrorResponse`**, una sola forma para toda la API. `Respond` y
  `RespondBindError` la devuelven.
- **`host` no se declara**, así que la UI usa el origen desde el que se sirve y no queda mal
  apuntada cuando cambia `API_PORT`.

Las validaciones de los DTOs viajan solas: swag lee los tags `binding` y los convierte en
`required`, `enum`, `minLength` y `maxLength` del spec. Un `binding:"oneof=MANUAL LEARNED"`
aparece como `enum` — otra razón para que la validación viva en el tag y no en el handler.
