# Autenticación

Access token corto más refresh token rotativo de un solo uso. La API es bearer: el
frontend es el que guarda el access token en una cookie httpOnly.

## Endpoints

| Método | Ruta                      | Auth                                       |
| ------ | ------------------------- | ------------------------------------------ |
| `POST` | `/v1/public/auth/login`   | no                                         |
| `POST` | `/v1/public/auth/refresh` | no (el token de refresco es la credencial) |
| `POST` | `/v1/auth/logout`         | sí                                         |

`login` y `refresh` devuelven el mismo cuerpo: `access_token`, `access_expires_at`,
`refresh_token`, y un `user` con `id`, `account_id` y `role`. **El refresh token se
muestra una sola vez** — solo se guarda su hash.

## Access token

JWT firmado con HS256, y el algoritmo está **fijado en la verificación**: un header
forjado con `alg: none` no puede degradar el chequeo.

Claims: `sub` (usuario), `account_id`, `role`, `session_epoch`, `iat`, `exp`.

**La sucursal activa no es un claim.** El vendedor cambia de sucursal sin volver a
loguearse, así que viaja en el header `X-Branch-Id` y se resuelve por request. Un valor
que no parsea se ignora: el que llama está operando a nivel cuenta, que es lo que hace
un admin.

## Refresh token

32 bytes de entropía, guardado solo como SHA-256 en hex. El valor crudo no es
adivinable, así que un hash rápido alcanza — a diferencia de una contraseña, donde hace
falta uno lento.

Cada token pertenece a una **familia** (`family_id`). Al refrescar se consume el
presentado y se emite el sucesor en la misma familia, todo en una transacción: un crash
en el medio no puede dejar una sesión sin ningún token vivo.

**Detección de robo con ventana de gracia:**

| Situación                                                            | Resultado                                                       |
| -------------------------------------------------------------------- | --------------------------------------------------------------- |
| Token vigente                                                        | rota normalmente                                                |
| Token ya consumido, **dentro** de `AUTH_REFRESH_REUSE_GRACE_SECONDS` | rotación fresca — es una carrera entre dos pestañas, no un robo |
| Token ya consumido, **más allá** de la ventana                       | se revoca **toda la familia** y devuelve 401                    |
| Token revocado, vencido o desconocido                                | 401                                                             |

Sin la ventana de gracia, tener dos pestañas abiertas te desloguea.

Una rotación **preserva la duración original**: una sesión "recordarme" no se degrada
sola al TTL corto.

## Logout inmediato

`app_user.session_epoch` se incrementa, y todo access token que lleve un epoch viejo
queda rechazado — sin lista negra. Además se revoca la familia del refresh token
presentado.

El refresh token en el cuerpo es **opcional**: un cliente que lo perdió tiene que poder
cerrar la sesión igual. Un token que pertenece a **otro** usuario se ignora en vez de
revocarse, así que logout no sirve para cerrarle la sesión a un tercero.

Precio del logout inmediato: el middleware hace **una lectura por PK indexada en cada
request autenticado** para comparar el epoch guardado. Es deliberado.

## Login

1. Busca el usuario por email en el **pool del owner**. Es obligatorio: en el login
   todavía no se sabe la cuenta, así que una query con scope de tenant no matchearía
   ninguna política y **fallaría siempre**.
2. Email desconocido, contraseña incorrecta y usuario inactivo devuelven **lo mismo**
   (401). Con un email que no existe igual se corre una comparación bcrypt contra un hash
   dummy, así que la latencia no delata qué direcciones están registradas.
3. El chequeo de `is_active` va **después** de la contraseña, para que tampoco se puedan
   enumerar cuentas deshabilitadas.
4. Contraseña incorrecta incrementa `failed_attempts`; al llegar a
   `AUTH_MAX_FAILED_ATTEMPTS` se setea `locked_until`.
5. Una cuenta bloqueada devuelve **429**, no 401 — eso sí se expone a propósito: el
   cliente necesita distinguir "contraseña incorrecta" de "dejá de intentar un rato".
   Mientras está bloqueada, la contraseña correcta también devuelve 429.

## El orden del middleware

1. `Authenticate` corre en todo `/v1`: verifica el token, chequea el epoch y setea el
   tenant. Un request **sin** header pasa sin autenticar en vez de ser rechazado, así una
   ruta pública puede ver quién llama cuando además está logueado.
2. `RequireTenant` guarda el grupo autenticado y devuelve 401 si no hay tenant.
3. `RequireAdmin` va después de `RequireTenant` en las rutas de admin.

## Contraseñas

bcrypt con el cost por defecto. Es una constante criptográfica, así que **no** es
configurable por entorno (a diferencia de los umbrales operativos).

Los dos usuarios del seed de desarrollo tienen la contraseña `coti1234`. Solo para
desarrollo.

## Configuración

Todo en `apps/api/.env.example`, con default en `internal/config`:
`AUTH_JWT_SECRET` (requerido, mínimo 32 caracteres), `AUTH_ACCESS_TTL_MINUTES`,
`AUTH_REFRESH_TTL_HOURS`, `AUTH_REFRESH_REMEMBER_DAYS`,
`AUTH_REFRESH_REUSE_GRACE_SECONDS`, `AUTH_MAX_FAILED_ATTEMPTS`,
`AUTH_LOCKOUT_MINUTES`.

## Pendiente

No están implementados todavía: recuperación de contraseña, invitación de usuarios y
verificación de email. Tampoco existe alta de cuentas — el seed es el único camino para
tener un usuario, y el bootstrap de cuenta va por `db.AdminTx()` porque la cuenta no
existe cuando se la crea.
