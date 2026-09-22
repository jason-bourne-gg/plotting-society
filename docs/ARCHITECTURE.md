# Architecture

How this is put together, and what to do when you add a feature.

---

## The shape

Every backend feature is one package under `internal/`, and every one of them
has the same three parts:

```
internal/<feature>/
    <feature>.go     types · Store (SQL) · Handler (HTTP) · Routes
```

`Store` owns SQL and returns domain errors. `Handler` owns HTTP: it parses,
validates, calls the guard, calls the store, and returns a typed `httpx.Error`.
Nothing else talks to the database, and nothing else decides a status code.

```
cmd/api/main.go          wires every module and starts the server
internal/access/         "may this caller act on this record?" — one file to audit
internal/auth/           Argon2id, JWT, invites, sessions, role middleware
internal/config/         env loading; fails at boot, never mid-request
internal/database/       pgx pool, embedded migrations, the pooler settings
internal/domain/         roles, statuses, the SLA table, shared validators
internal/httpx/          typed errors, JSON, middleware
internal/testsupport/    per-package test database and fixtures
```

Modules never import each other. They share `domain`, `httpx`, `database`,
`access` and `auth` — all of which are leaf-ish and have no opinion about any
particular feature. That is what keeps a new module from dragging the rest of
the app with it.

---

## Adding a feature

Concretely, using "documents" as the example.

**1. Write the migration.** `internal/database/migrations/00NN_documents.sql`.
They are plain SQL applied in filename order inside a transaction, and they run
themselves when the container boots. Never edit a migration that has shipped.

Put the rules the database can enforce *in* the database — a `CHECK` on a status
column, a `UNIQUE` on a natural key, a foreign key with the right `ON DELETE`.
A handler can be wrong; a constraint cannot be bypassed.

**2. Write the package.**

```go
type Store struct{ db *database.DB }
func NewStore(db *database.DB) *Store { return &Store{db: db} }

type Handler struct {
    store *Store
    guard *access.Guard
}
func NewHandler(store *Store, guard *access.Guard) *Handler { ... }

func (h *Handler) Routes(mux *http.ServeMux, a *auth.Authenticator) {
    mux.Handle("GET /api/societies/{societyId}/documents", a.Optional(httpx.Handler(h.list)))

    staff := func(fn httpx.Handler) http.Handler {
        return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
    }
    mux.Handle("POST /api/societies/{societyId}/documents", staff(h.create))
}
```

**3. Scope every staff route through the guard.** `RequireStaff` proves the
caller is *a* builder. It does not prove they are *this* builder, and every
route takes an id straight from the URL:

```go
caller := auth.MustFromContext(r.Context())
if err := h.guard.Society(r.Context(), caller.Role, caller.BuilderID, societyID); err != nil {
    return err
}
```

The guard returns **404, not 403**, on a miss. A 403 confirms the record exists,
which turns an id into an oracle for enumerating other builders' data.

**4. Wire it in `cmd/api/main.go`** — one line next to the others.

**5. Test it.** See below.

**6. Add it to `test/smoke.mjs`** if it has a page.

---

## Rules that are not negotiable

**Money is `numeric`, never float.** And the fund ledger is append-only: a
correction writes a reversal row linked to the original. When an owner disputes
a figure the builder shows the history, not a number that quietly changed.

**A bill snapshots what it was computed from.** `maintenance_dues` stores the
rate, the area and the sector alongside the amount. Changing a rate must never
re-price an invoice already issued.

**jsonb columns are `json.RawMessage`, never `[]byte`.** pgx scans jsonb into a
byte slice correctly, but `encoding/json` marshals a plain `[]byte` to a base64
*string* — so the client gets `"W3sibGFiZWwi..."` where it expects an array, and
the page dies on `.map`. There are tests asserting the wire format for exactly
this reason.

**Parameters that Postgres cannot type need an explicit `::cast`.** Production
runs behind a transaction-mode pooler, which forces `QueryExecModeExec`, which
means pgx infers parameter types itself instead of asking the server. A uuid
slice needs `[]string` plus `::uuid[]`; a bare boolean predicate needs
`$n::boolean`. See the long note in `internal/database/database.go` — all three
obvious pgx modes fail on a pooler, each differently, and one of them fails
*only under concurrency*.

**Owner-facing data is scoped by ownership, not by obscurity.** The layout map
carries no names or phone numbers at all; they appear only on a plot's own
detail page, to that plot's owner and to the builder.

---

## Testing

Store and handler tests run against a **real Postgres**, not mocks. These
layers are mostly SQL, and a mock will agree that a query filters by
`builder_id` when it does not.

```go
db := testsupport.DB(t, "documents")   // own database, migrated, empty
b := testsupport.NewBuilder(t, db, "Alpha")
```

Every tenancy check is tested three ways — the owning builder, a *different*
builder, and a super admin — because a check that is present but wired to the
wrong column still passes a single happy-path test.

```bash
make up && make test        # Go, needs the local Postgres
make test-docker            # same, without a local Go install
make cover                  # coverage by package
make smoke                  # schema rules, in SQL
node test/smoke.mjs <url>   # all routes in a real browser, as all three roles
```

CI runs all of this on every push, with a coverage floor of 85%.

The browser test asserts something only a correct render produces — the map
counts plots, the plot page checks the bill shows its working. "The page is not
blank" passes a layout map with no plots on it, which is exactly the bug it
failed to catch the first time.

---

## Frontend

```
web/src/
    lib/api.ts          one fetch wrapper; token refresh is transparent
    lib/auth.tsx        session context
    lib/useSociety.tsx  which project is being shown — a selection, not a constant
    components/         shared UI; PlotMap is used by both the owner and guest maps
    pages/              one file per route
    pages/admin/        the builder portal
```

**There is no "the society".** A builder runs several projects, so everything
reads `useSociety()` and the shell offers a switcher when there is more than
one. The first version pinned `societies[0]` in a module-level variable; adding
a second project would have silently shown the wrong one.

**Anything that reaches an `href` goes through `safeUrl()`.** Attachment and
document URLs are accepted as strings so an upload can be presigned and PUT in
one round trip, which also accepts `javascript:` — stored XSS the moment someone
clicks it. The server rejects those on write; `safeUrl` is the second lock.

---

## Known limits

Honest list of what will need work as this grows, and roughly when.

| Limit | Bites at | Fix |
|---|---|---|
| List endpoints return everything | a few thousand plots per society | keyset pagination on `(created_at, id)`; the clients already pass `limit` |
| `audit_log` exists but nothing writes to it | the first ownership dispute | an `audit` helper called from fund, plot status and invite acceptance |
| Rate limiting is only on the guest enquiry | the first scripted login attempt | the same limiter, keyed on account, in front of `/api/auth/login` |
| The limiter is per instance | more than one API instance | move the counter to the database or Redis |
| No API versioning | the first breaking change with a released mobile app | prefix `/api/v2`; the router already matches on method and path |
| Photos have no resize step | owners on mobile data | resize on upload with a Worker, or accept a `srcset` from the client |

None of these are load-bearing yet at 823 plots and one society. They are listed
so the next person does not have to rediscover them.
