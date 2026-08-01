# API specification

The contract is **generated from the handler annotations** with
[swaggo/swag](https://github.com/swaggo/swag). It lives next to the code that implements it,
so it goes stale less often than a hand-written YAML.

```bash
pnpm docs:api          # regenerate apps/api/docs/
pnpm docs:api:check    # regenerate and fail if the result differs from what is committed
```

With the API running, the UI is at **http://localhost:8000/swagger/index.html** and the raw
spec at `/swagger/doc.json`.

## The generated files are committed, and CI compares

`apps/api/docs/` (`docs.go`, `swagger.json`, `swagger.yaml`) is in git. The
**"OpenAPI spec is up to date"** step in `ci.api.yml` regenerates and runs
`git diff --exit-code`: touch a handler without regenerating and CI fails.

That step is the whole point. **An annotation nothing compiles is a comment**, and it drifts
from the code with nothing to say so. The diff is what makes it verified.

Two consequences to respect:

- **Prettier does not touch `apps/api/docs/`** (it is in `.prettierignore`). Formatting it
  would make the committed copy differ from what swag emits, and CI compares exactly those
  two things.
- **The CLI is pinned in `go.mod`** with Go 1.24+'s `tool` directive (`go tool swag`), not a
  floating `@latest`: two people on different versions generate different specs and the diff
  becomes noise.

## Why swag v1 and not v2

v2 emits OpenAPI 3.1, which is what you would want. But it sits at **`v2.0.0-rc5`** — a
release candidate — and `gin-swagger/v2` **has no published version at all**, so there is no
stable way to serve it. Pinning the API contract's generator to an RC with no serving
companion is not worth 3.1.

v1 emits Swagger 2.0, which every tool reads, and the annotation syntax is nearly identical,
so migrating when v2 stabilizes is cheap. Revisit then.

## The spec is not published in production

The `/swagger/*any` route is mounted only when `ENV != production`. It describes an internal
API: publishing it would hand an unauthenticated reader the whole surface for nothing. The
consumers are this repo's two web apps and whoever is writing them.

## Annotating a handler

The block goes in the doc comment, indented with tabs so `gofmt` treats it as preformatted
and leaves it alone:

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

Rules already settled:

- **`@Router` carries the full path**, `/v1` included. `basePath` is `/` because the probes
  (`/health`, `/ready`) live outside `/v1` and a basePath of `/v1` would document them wrong.
- **`@Security BearerAuth`** on every route that needs a session; the `/v1/public` ones do
  not carry it.
- **Errors are always `dto.ErrorResponse`**, one shape for the whole API. `Respond` and
  `RespondBindError` both return it.
- **`host` is not declared**, so the UI uses whatever origin serves it and does not go stale
  when `API_PORT` changes.
- **Keep `@Description` to one line.** Changing one means regenerating `apps/api/docs/` in
  the same commit, or the drift check fails.

DTO validations travel on their own: swag reads the `binding` tags and turns them into the
spec's `required`, `enum`, `minLength` and `maxLength`. A `binding:"oneof=MANUAL LEARNED"`
shows up as an `enum` — one more reason for validation to live in the tag rather than the
handler.
