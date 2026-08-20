# File storage

Attachments and quote documents are bytes the database only points at. They go through one port,
and the adapter behind it is a startup decision; nothing above the port knows which one answered.

## The port and its adapters

`domain.ObjectStorage` is the port — `Upload`, `Download`, `GenerateSignedURL`. Adapters live in
`apps/api/internal/storage/`, and `internal/storage/provider` is the only place one is bound.

| Provider | Behaviour                                                                        |
| -------- | -------------------------------------------------------------------------------- |
| `local`  | Keeps objects on the filesystem and serves them through the API's own link route |
| `spaces` | Stores them in an S3-compatible bucket, which signs and serves its own links     |

`local` is the default, and it genuinely works: a checkout with no bucket — which is every
checkout until a Space is paid for — uploads, downloads and expires links exactly as the
deployed one will. There is no stand-in that accepts a file and stores it nowhere. An in-memory
adapter exists only inside `fake_test.go`, where no binary can reach it.

Selecting `spaces` with a credential missing fails at startup, every blank key named in the same
pass rather than one restart per problem.

## The content type travels with the object

`Upload` takes the content type because `Download` has to give it back: a signed link serves the
bytes as whatever the storage says they are, and a PDF served as `text/plain` opens as mojibake.
A bucket keeps that natively. The local adapter has nowhere on a filesystem to put it, so it
writes a `.meta` file beside the object holding exactly that string — which is why a storage key
may not itself end in `.meta`.

## Keys are canonical, relative, and account-first

The account goes at the front of the key, so isolation is visible in the object's own path:

```
accounts/<account_id>/rfqs/<rfq_id>/<object_id>.<ext>
```

One key cannot be a prefix of another on the local adapter: a filesystem cannot hold both a file
at `a/b` and a directory at `a/b/c`, where a bucket holds both happily. Keys ending in an object
id sidestep it, which is what the layout above does.

The key rules belong to the port, not to one adapter: **both** refuse the same keys, so an object
stored through one is reachable through the other. A key that is not already canonical — absolute,
or carrying `.`, `..`, a doubled or trailing separator — is refused because a bucket stores
`a/./b` verbatim where a filesystem resolves it, and the two would then disagree about where an
object lives. A key that climbs out of the base directory is refused separately, because
`path.Clean` leaves a leading `../` exactly where it found it. Empty keys and keys ending in
`.meta` are refused for the reasons above.

On top of that, the local adapter checks the **resolved** path landed inside its base directory
rather than trusting the key rules to have covered it. Nothing reaches that check today, so no
test pins it; it is there because `filepath.Join` is platform-dependent and the key rules are not.

## What a signed link is, per adapter

`spaces` returns a presigned GET URL. The bucket validates it and serves the object; the API
never carries the bytes on the way out.

`local` returns a link at `/v1/files/<key>` carrying two query parameters:

```
?expires=<unix seconds>&signature=<base64url HMAC-SHA256>
```

The signature covers the key and the deadline together, over `STORAGE_SIGNING_SECRET`. Neither
can be moved without invalidating the other: pushing `expires` further out, or pointing the same
signature at another account's key, both fail the comparison. Verification is stateless — no
table, no lookup — and the deadline is the first instant that no longer serves, not the last one
that does.

Both refusals answer 403. Only the error code separates them: `LINK_EXPIRED` for a deadline that
has passed, `INVALID_LINK` for a signature that never covered the request. That distinction is
safe to make — whoever holds an expired link already held a valid one — and it is what lets a
client offer a fresh link for the one case where that helps.

### What the route sends back

The bytes are a client's and the origin is the API's own, so a served object must never be able
to run as a page on it. Every response carries `X-Content-Type-Options: nosniff`, so the browser
takes the stored content type rather than guessing a better one, and `Content-Disposition:
attachment`, so it downloads rather than renders in place. `Cache-Control: private, no-store`
keeps a shared cache from holding a body past the deadline the signature exists to enforce.

## Configuration

In `apps/api/.env.example`, defaults in `internal/config`:

| Key                                 | Meaning                                                             |
| ----------------------------------- | ------------------------------------------------------------------- |
| `STORAGE_PROVIDER`                  | `local` (default) or `spaces`                                       |
| `STORAGE_MAX_FILE_SIZE_BYTES`       | refused before any byte is stored; defaults to 10 MiB               |
| `STORAGE_SIGNED_URL_EXPIRY_MINUTES` | how long a link serves; defaults to 15                              |
| `STORAGE_LOCAL_DIR`                 | where objects live; required when the provider is `local`           |
| `STORAGE_LOCAL_API_BASE_URL`        | where signed links point — this API's own address, not a frontend's |
| `STORAGE_SIGNING_SECRET`            | signs local links; at least 32 characters, required under `local`   |
| `STORAGE_ENDPOINT`                  | required when the provider is `spaces`; carries no bucket name      |
| `STORAGE_REGION`                    | required when the provider is `spaces`                              |
| `STORAGE_BUCKET`                    | required when the provider is `spaces`                              |
| `STORAGE_ACCESS_KEY`                | required when the provider is `spaces`                              |
| `STORAGE_SECRET_KEY`                | required when the provider is `spaces`                              |

The signing secret is a credential, not a convenience: a link is only as private as that value,
so a deployment sharing one shares every file it stores. It is demanded under `local` for the
same reason `AUTH_JWT_SECRET` is demanded at all, and to the same 32-character floor — the same
HMAC construction needs the same key width.

Each adapter is handed only its own settings. `StorageConfig.Local()` carries the directory, the
link base and the signing secret; `StorageConfig.Spaces()` carries one bucket's coordinates.
Neither can reach the other's credential, so no `%+v` puts a bucket key in a filesystem adapter's
log line.

### Pointing it at DigitalOcean Spaces

Three settings there are not what they look like:

- **`STORAGE_REGION` is `us-east-1` whichever datacenter holds the Space.** The region is used
  for signing and validation only; the endpoint is what routes. A real DigitalOcean region here
  makes the SDK sign a different payload and bucket operations fail.
- **`STORAGE_ENDPOINT` carries no bucket name** — `https://nyc3.digitaloceanspaces.com`. The SDK
  prepends the bucket itself.
- **Spaces keys come from API → Spaces Keys**, and are not a DigitalOcean personal access token.

Objects are stored private and reached only through presigned links, which is the only ACL
arrangement worth having: the alternative Spaces offers is `public-read`.

## In development

`local` needs nothing. Objects land in `STORAGE_LOCAL_DIR` (`./.storage`, gitignored) and the
API serves them itself. Under `pnpm dev:docker` the directory is a named volume, so files survive
a rebuild instead of leaving the database pointing at objects that no longer exist.
