# Integration test suite — design

Status: **proposal for review**. Nothing here is implemented yet.

The maintainer asked for "an integration test suite that verifies the API on a
running server and which is run during the CI tests. This should ensure that
the API behavior does not change unless we want it to."

Beacon's "API" is the HTTP surface of a server-rendered app: routes and
methods, login and role checks, redirects, status codes, form validation
behaviour, the CSV export, file downloads (Document 37, receipts) and the
health endpoints. This document proposes how to lock that surface down.

## 1. Goals and non-goals

Goals:

- Detect any unintended change to the HTTP surface: a route added, removed or
  renamed; a role gaining or losing access; a different status code or
  redirect target; a changed CSV.
- Test the **real binary** (`cmd/beacon`) with a real Postgres, the way it runs
  in production, including migrations and bootstrap.
- Each run is **isolated** (its own database) and safe to run **in parallel**.
  The suite must not add to the `go test -p 1` problem.
- Runs on a laptop (`make test-integration`) and in CI on every PR.
- Intentional changes are cheap: re-run with `-update`, review the golden diff
  in the PR.

Non-goals:

- Not a browser or UI test. No headless Chrome, no JavaScript, no visual
  checks. `receipt-resize.js` stays untested beyond "served".
- Not a replacement for unit tests in `internal/server`, `internal/report`,
  `internal/store`. Edge cases of validation and report arithmetic stay there;
  the integration suite checks one representative case per behaviour.
- No production or staging data, no network access beyond local Postgres, no
  Railway.
- No performance or load testing.

## 2. Route inventory and authorization matrix

This is the core of "the API does not change unless we want it to". Two
golden files, both checked into git.

### 2.1 Route inventory (unit test, no database)

Today `Server.Handler()` registers routes directly on an `http.ServeMux`, and
the mux cannot list its patterns. Proposal: a small refactor so the routes are
declared once in a table and `Handler()` loops over it.

```go
type Guard string

const (
	GuardPublic Guard = "public"
	GuardLogin  Guard = "login"    // requireLogin
	GuardRHC    Guard = "rhc_admin" // requireRole(RoleRHCAdmin)
)

type Route struct {
	Pattern string // "GET /occupants/{id}/edit"
	Guard   Guard
	handler http.Handler
}

func (s *Server) Routes() []Route { ... }   // single source of truth

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.Routes() {
		mux.Handle(rt.Pattern, s.guard(rt.Guard, rt.handler))
	}
	return mux
}
```

A plain unit test (`internal/server/routes_test.go`, no database) writes
`Routes()` as text and compares it with `internal/server/testdata/routes.golden`:

```text
GET  /                                   public
GET  /account                            login
POST /account/passkeys/{cred}/delete     login
GET  /admin/users                        rhc_admin
...
```

Adding, removing or re-guarding a route fails this test until the golden file
is regenerated with `go test ./internal/server -run TestRoutes -update`. The
diff shows up in the PR. The alternative, parsing `server.go` with `go/ast`,
avoids the refactor but breaks as soon as registration code changes shape, so
it is not recommended.

### 2.2 Authorization matrix (integration test)

The inventory only knows the middleware guard. Most scoping in Beacon happens
inside handlers (`canAccessHouse`, `canConfirmMonth`, `canReviewExpenses`,
`loadExpenseInScope`, the RHC-only check in `handleDashboard`). The matrix
test therefore sends real requests to the running binary.

**Fixture** (seeded once per matrix run):

- RHL **A** with houses **A1** and **A2**; RHL **B** with house **B1**.
- Users: `rhc` (rhc_admin), `rhlA` (rhl_admin of A), `mgrA1`
  (safe_house_manager of A1). All active, each with a session.
- In A1 and B1: one occupant, one expense with a receipt, one expense without,
  one operational issue, one safeguarding concern, one ended month with data.
- Sacrificial users for `lock`, `reinvite`, passkey delete, so those POSTs can
  succeed without breaking later rows.

**Actors** (columns):

| Column | Meaning |
|---|---|
| `anon` | no cookie |
| `mgr` | `mgrA1`, target in A1 |
| `mgr.x` | `mgrA1`, target in A2 (same RHL, other house) |
| `rhl` | `rhlA`, target in A1 |
| `rhl.x` | `rhlA`, target in B1 (other RHL) |
| `rhc` | `rhc`, target in A1 |

For routes without a house-scoped target the `.x` columns are omitted.

**Probes.** Each route has one or more probes. A probe fills path parameters
from the fixture (`{id}` → the occupant/expense/issue/concern of the target
house, `{houseID}` → target house, `{file}` → `2026-01.csv`, `{month}` → the
ended month, `{token}` → a fresh invite, `{cred}` → the actor's own passkey).
Routes with query-string scoping get extra probes, e.g.
`GET /reports?house={houseID}`. POST probes send a deliberately **minimal
form** (usually empty) so an authorized request reaches the handler and fails
validation, rather than changing data. Where an empty POST does change data
(lock, reinvite, depart, delete), the probe targets a sacrificial record.

**Recorded outcome** per cell: status code, plus the `Location` path (query
string dropped, IDs replaced by `{id}`) for redirects. Bodies are not
recorded. The golden file (`integration/testdata/authz.golden`) looks like:

```text
GET  /occupants/{id}/edit     anon=303:/login mgr=200 mgr.x=403 rhl=200 rhl.x=403 rhc=200
POST /occupants/{id}/depart   anon=303:/login mgr=303:/occupants/{id}/edit mgr.x=403 ...
GET  /dashboard               anon=303:/login mgr=403 rhl=200 rhc=200
GET  /admin/users             anon=303:/login mgr=403 rhl=403 rhc=200
```

(Values are illustrative; the first `-update` run records today's behaviour,
and the maintainer reviews that first golden file line by line. That review
is also a one-off security audit of the current authz.)

**Completeness check:** the test fails if the set of patterns in
`authz.golden` differs from `server.Routes()`. A new route cannot ship
without a deliberate matrix entry.

**Also recorded, cheaply:** one wrong-method request per path (expect `405`
with the `Allow` header from Go's mux) and one unknown path (`404`).

## 3. Scenario tests against the real binary

The matrix proves "who may call what". Scenarios prove "what happens when they
do", end to end, through HTTP only: no direct store calls after the initial
login setup.

### 3.1 Harness

```go
//go:build integration

func TestMain(m *testing.M) {
	// go build -o $TMP/beacon ./cmd/beacon  (once per package run)
}

type App struct {
	URL    string       // http://127.0.0.1:54321
	DB     *sql.DB      // for fixture seeding only
	Log    *bytes.Buffer // server stdout/stderr, dumped on failure
}

func Start(t *testing.T) *App // fresh database + process, cleaned up via t.Cleanup
```

`Start`:

1. Creates a database `beacon_it_<random>` (section 4).
2. Starts the binary with `ADDR=127.0.0.1:0`, `DATABASE_URL` pointing at it,
   `WEBAUTHN_RP_ID=localhost`, `WEBAUTHN_RP_ORIGINS=http://localhost`,
   `BOOTSTRAP_ADMIN_EMAIL=rhc@example.com`, `SECURE_COOKIES=false`.
3. Reads the real listen address from the log and polls `/readyz` (10 s
   budget). On failure the test prints the server log.
4. On cleanup sends `SIGTERM`, waits, and drops the database.

**Small production change needed:** `runServer` logs `httpSrv.Addr`, which is
`:0` with a random port. Switch to `net.Listen` + `httpSrv.Serve(ln)` and log
`ln.Addr()`. This is harmless in production and avoids a pick-a-free-port race.

**WebAuthn on a random port works already.** The server compares the origin
inside `clientDataJSON` with `WEBAUTHN_RP_ORIGINS`; it never looks at the TCP
port. `virtualwebauthn` writes whatever origin we give it, so the harness uses
`http://localhost` for both, exactly as `internal/server/auth_flow_test.go`
does today with `httptest`.

**Login per role over HTTP:**

```go
admin := app.Bootstrap(t)                 // invite_url from server log → register → login
rhl   := admin.Invite(t, "rhl@example.com", "rhl_admin", rhlA)  // POST /admin/users/invite,
mgr   := admin.Invite(t, "mgr@example.com", "safe_house_manager", houseA1) // token from invite page
```

Each returned `Client` has its own cookie jar, its own virtual authenticator,
and helpers `Get`, `PostForm`, `PostMultipart` that **do not follow
redirects**, so every test can assert status and `Location`. The existing
`register`/`login` helpers in `auth_flow_test.go` move into a small shared
package, e.g. `internal/testutil/webauthnclient`, used by both unit and
integration tests.

For the matrix only, users and sessions may be seeded directly with
`store.CreateUserWithInvite` + `store.CreateSession` to keep it fast (60+
routes × 6 actors). Scenarios always log in through WebAuthn.

### 3.2 Scenarios (initial set)

| Scenario | Key assertions |
|---|---|
| **Bootstrap and health** | `/healthz` = `ok`, `/readyz` = `ready`, `/login` 200, bootstrap invite URL in log, invite page 200, reused invite rejected. |
| **Admin lifecycle** | Invite RHL admin and manager; edit role and email; lock → login begin fails and existing session is rejected; reinvite → pending → register again; remove a passkey (admin and self), last-passkey rule; self-lock refused with `cannot modify yourself`. |
| **Occupants** | Create (303 to list), duplicate nickname in same house rejected with an error redirect, same nickname in another house allowed, rename to a taken nickname rejected, demographics update, depart with a date before arrival rejected, depart valid, arrival too far in the future rejected. |
| **Expenses** | Create without receipt requires an explanation; create with `testdata/receipts/mattress.png` (multipart); `GET /expenses/{id}/receipt` returns the same bytes, content type, `Content-Disposition: inline`; oversize receipt rejected; review by RHL admin, review by manager refused; edit and delete. |
| **Operations** | Create, edit, close an operational issue; future identified date rejected. |
| **Safeguarding** | Create a concern, update status to closed with a closing date; `GET /safeguarding/document-37` returns the docx content type, `Content-Disposition` filename and a non-empty body starting with the ZIP magic `PK`. |
| **Monthly report** | Seed occupants and expenses for a fixed past month; `/reports?house=…&month=…` shows the expected bed-nights, demographic counts, expense totals and receipt counts (by markers, see 3.3); CSV compared with a golden file. |
| **Month confirmation** | Confirm an ended month (manager and RHL admin succeed, RHC admin refused); stale fingerprint rejected with the "data changed" message; changing an expense's merchant/category/no-receipt reason reopens the month (history page shows it as changed); current month cannot be confirmed. |
| **Dashboard and history** | RHL admin sees only RHL A houses; RHC admin sees all; history lists 12 ended months. |

Each scenario is one top-level `Test…` with `t.Parallel()` and its own `App`.

### 3.3 What to compare, and what not

Compared:

- **Status code** and **normalized `Location`** on every request.
- **Headers that are part of the contract:** `Content-Type`,
  `Content-Disposition`, `Cache-Control: no-store` on HTML/CSV/receipt,
  `Set-Cookie` attributes on login (HttpOnly, SameSite=Lax, Path).
- **Flash and error messages** that users act on (`?error=` text in the
  `Location`, visible error text in the page).
- **Stable HTML markers**, found with `strings.Contains` or a tiny helper:
  e.g. a row for the nickname, `aria-current="page"` in navigation, a total
  like `CAD 123.45`. If templates change often, add `data-test="…"`
  attributes on the few values we assert (maintainer decision).
- **CSV exports: full content** against a golden file, after masking the
  `Generated` timestamp row.
- **Downloads:** exact bytes for receipts (we uploaded them), content type,
  size and magic bytes for Document 37 (the file is embedded; its hash is
  already fixed by the build).

Not compared: full HTML snapshots. Every template edit, CSS class rename,
asset hash (`?v=…`) or version string in the footer would break them. People
learn to run `-update` without reading the diff, and the golden files stop
protecting anything. HTML layout is not part of the contract; the numbers,
messages, redirects and access rules are.

## 4. Test data and isolation

### 4.1 One database per test

The CI and Compose Postgres user `beacon` is the container superuser, so it
may create databases. Per `App`:

```sql
CREATE DATABASE beacon_it_3f9a1c TEMPLATE beacon_it_template;
-- ... test runs, then:
DROP DATABASE beacon_it_3f9a1c WITH (FORCE);
```

`beacon_it_template` is created once per package run by running the binary's
migrations against an empty database (or `migrate.Up` in `TestMain`), keyed by
a hash of `internal/migrate/sql/*` so a stale template is rebuilt. Copying a
template takes tens of milliseconds; migrating from scratch per test would
also be acceptable at today's 18 migrations. `TestMain` also drops leftover
`beacon_it_%` databases older than an hour, in case a previous run was killed.

Consequences:

- Tests never touch the `beacon` database, so they cannot destroy a local
  dev dataset (unlike `verify-phase1.sh`, which drops tables in it).
- Scenarios run with `t.Parallel()`; `go test` may run the integration
  package alongside others. No `-p 1`.
- Test data is synthetic (nicknames like `it-occ-1`, `example.com` emails).
  No real names, per AGENTS.md.

### 4.2 Time

"Today" matters in many places: arrival/identified/safeguarding date limits,
report month limits, "ended month" for confirmation, the dashboard's last
ended month, history windows. It is read both in Go (`time.Now()` in
handlers) and in SQL (`CURRENT_DATE` in `internal/store/occupants.go`, `now()`
for invite expiry).

**Recommended for phases 1–2: avoid a test clock.**

- Scenarios compute dates relative to the real UTC today once at the start
  (`today`, `today+5`, `lastEndedMonth`).
- Report and CSV goldens use a **fixed past month** (e.g. `2026-01`). Data for
  an ended month does not depend on "now", except the `Generated` row, which
  is masked. If a golden differs only because a fixed month fell out of some
  window, that is a real behaviour change worth seeing.
- Guard against month-boundary flakes: if the UTC month or day changed
  between the start and end of a test, the test calls `t.Skip` and logs why,
  instead of failing.

**Option for later: an injected clock**, only if a scenario truly needs a
specific "today" (e.g. deadline banners on the dashboard):

- `Server` gets a `now func() time.Time`; handlers use `s.now()`.
- SQL stops using `CURRENT_DATE`; the date is passed as a parameter.
- The binary reads `BEACON_TEST_NOW=2026-02-03T10:00:00Z` **only when built
  with `-tags beacon_testclock`**. Production builds do not contain the code,
  so the variable cannot be abused in production.

This is a moderate refactor across handlers and store; the design keeps it
optional.

### 4.3 WebAuthn configuration

`WEBAUTHN_RP_ID=localhost`, `WEBAUTHN_RP_ORIGINS=http://localhost`, requests
go to `http://127.0.0.1:<port>`. Cookies are not `Secure`. See 3.1 for why the
port does not matter.

## 5. Layout, commands, CI

```text
integration/
  doc.go                 //go:build integration
  harness_test.go        // build binary, Start(), DB lifecycle
  client_test.go         // Client: no-redirect HTTP, forms, multipart, markers
  authz_test.go          // matrix
  scenario_*_test.go     // one file per area
  testdata/
    authz.golden
    report-2026-01.csv.golden
internal/server/testdata/routes.golden
internal/testutil/webauthnclient/   // shared register/login helpers
```

The build tag keeps `go test ./...` fast and database-free for this package.

Make targets:

```makefile
test-integration: db-up
	GOTOOLCHAIN=$(GOTOOLCHAIN) go test -tags integration -count=1 -timeout 5m ./integration/...

update-goldens:
	go test ./internal/server -run TestRoutes -update
	go test -tags integration -count=1 ./integration/... -update
```

CI (`.github/workflows/ci.yml`), same job and Postgres service, after unit
tests so a unit failure fails fast:

```yaml
      - name: Test
        run: go test -p 1 ./...
      - name: Integration test
        run: go test -tags integration -count=1 -timeout 5m ./integration/...
```

Time budget: under 2 minutes in CI (one `go build`, ~10 scenario processes in
parallel, matrix ≈ 400 requests against one process). Alternative: a separate
job with its own Postgres service that runs in parallel with unit tests;
faster wall clock, more YAML.

Optional: build the binary with `go build -cover` and set `GOCOVERDIR` to get
coverage of `internal/server` from the integration run.

**Updating goldens:** run `make update-goldens`, then `git diff` the
`testdata/` files. The PR description must say why the surface changed. A
changed line in `authz.golden` is a change in who can do what; reviewers
should read it like a permissions change.

## 6. Relationship to existing scripts and to `-p 1`

`scripts/verify-phase1.sh` … `verify-phase5.sh` each: run `go test -p 1
./...` (or a subset), start the binary on the fixed port 8080 against the
shared `beacon` database (phase 1 and 2 first drop its tables), and `curl` a
few routes for 200/303. Some also `grep` for files and text in docs.

The integration suite covers all of the HTTP checks, with isolation and
without port 8080. Proposal:

- Retire `verify-phase1.sh` … `verify-phase5.sh` and their make targets once
  phase 1 of this plan lands. Move the few file/text greps that still matter
  (OPERATOR.md sections, templates present) into a unit test or drop them.
- Keep `verify-phase6.sh` as `make verify-deploy` (Docker image build, deploy
  compose config, Railway config checks). The integration suite does not
  build images.
- Keep `verify-bootstrap.sh` (Compose full-stack smoke) and `verify-cla.sh`.
- Update AGENTS.md and README.md accordingly.

**Aside: dropping `-p 1` for unit tests.** The same per-database approach
works for existing tests. Change `testDB` (in `internal/server`,
`internal/store`, `internal/migrate`) to create `beacon_test_<package>_<pid>`
on first use and drop it at the end of `TestMain`. Packages then no longer
share a schema and `go test ./...` can run them in parallel. Tests inside
`internal/server` still reset the schema between tests and stay sequential
within the package; that is fine. This is a separate small PR and not
required for the integration suite.

## 7. Risks, costs, rollout

Risks and costs:

- **Runtime:** one binary build (~5–10 s cold) plus parallel processes.
  Acceptable within the 2-minute budget; watch it as scenarios grow.
- **Flakiness:** process start, readiness polling, parallel databases, day or
  month boundaries. Mitigations: `/readyz` polling with a clear timeout and
  log dump, no fixed ports, no sleeps, month-boundary skip, `-count=1`.
- **Golden maintenance:** the matrix changes whenever a route is added. That
  is the point, but it costs a minute per such PR. Keeping bodies out of the
  goldens keeps churn low.
- **Refactors:** route table in `server.go` and listener address logging in
  `main.go` are small. The optional test clock is not.
- **Duplication:** some existing `internal/server` HTTP tests overlap with the
  scenarios. Leave them; trim later only if they become a burden.

Phased rollout, one PR each:

1. **Route inventory + authz matrix.** Route table refactor,
   `routes.golden`, listener address change, harness, `authz.golden`, CI
   step, `make test-integration`. Retire `verify-phase1..5`.
2. **Core scenarios.** Shared WebAuthn helpers; bootstrap/health, admin
   lifecycle, occupants, expenses and receipts, operations, safeguarding and
   Document 37, confirmation and change detection.
3. **Report and CSV goldens.** Fixed-month dataset, report page markers,
   CSV golden, dashboard and history. Decide here whether the test clock is
   needed.

## 8. Open decisions for the maintainer

1. **Route table refactor** in `server.go` so the route inventory comes from
   the real mux (recommended), or parse `server.go` with `go/ast`?
2. **Time:** avoid a test clock (relative dates + fixed past months,
   recommended for now), or build the `beacon_testclock` seam including
   removing `CURRENT_DATE` from SQL?
3. **Real binary** (recommended, matches "running server") or in-process
   `httptest` with the same harness API (faster, no build, but skips
   `main.go`, env parsing and bootstrap)?
4. **Matrix seeding:** allow direct store seeding and session creation for
   the matrix (fast), or HTTP-only everywhere?
5. **HTML markers:** plain text matching, or add `data-test` attributes to
   the few asserted values?
6. **CI placement:** step in the existing job (simpler) or a parallel job
   (faster wall clock)?
7. **Scripts:** retire `verify-phase1..5`, rename `verify-phase6` to
   `verify-deploy`?
8. **Unit tests:** do the per-package database change to drop `-p 1` now,
   as a separate PR, or later?

## Appendix: routes today

From `Server.Handler()` in `internal/server/server.go` (module `github.com/RainbowHaven/beacon`, stacked on PR #30). *Guard* is the middleware: `public` = none, `login` = `requireLogin` (anonymous → `303 /login`), `rhc_admin` = `requireRole(RoleRHCAdmin)` (anonymous → `303 /login`, other roles → `403`). *In-handler checks* lists the notable scoping done inside the handler, which only the matrix test can see.

| Method | Path | Guard | In-handler checks |
|---|---|---|---|
| `GET` | `/static/` | public | cache headers when `?v=` is set |
| `GET` | `/healthz` | public |  |
| `GET` | `/readyz` | public |  |
| `GET` | `/{$}` | public | home page |
| `GET` | `/login` | public |  |
| `POST` | `/logout` | public |  |
| `GET` | `/invite/{token}` | public | invite token valid and unused |
| `POST` | `/webauthn/register/begin` | public | invite token (query/body) |
| `POST` | `/webauthn/register/finish` | public | invite token + challenge cookie |
| `POST` | `/webauthn/login/begin` | public |  |
| `POST` | `/webauthn/login/finish` | public | challenge cookie; user active |
| `GET` | `/help/passkeys` | public |  |
| `GET` | `/account` | login |  |
| `POST` | `/account/passkeys/begin` | login |  |
| `POST` | `/account/passkeys/finish` | login |  |
| `POST` | `/account/passkeys/{cred}/rename` | login |  |
| `POST` | `/account/passkeys/{cred}/delete` | login | own passkey; last-passkey rule |
| `GET` | `/admin/users` | rhc_admin |  |
| `GET` | `/admin/users/invite` | rhc_admin |  |
| `POST` | `/admin/users/invite` | rhc_admin |  |
| `GET` | `/admin/users/{id}/edit` | rhc_admin |  |
| `POST` | `/admin/users/{id}` | rhc_admin |  |
| `POST` | `/admin/users/{id}/lock` | rhc_admin | not self |
| `POST` | `/admin/users/{id}/reinvite` | rhc_admin | not self |
| `POST` | `/admin/users/{id}/email` | rhc_admin |  |
| `POST` | `/admin/users/{id}/passkeys/{cred}/delete` | rhc_admin |  |
| `GET` | `/admin/houses` | rhc_admin |  |
| `POST` | `/admin/rhls` | rhc_admin |  |
| `POST` | `/admin/rhls/{id}` | rhc_admin |  |
| `POST` | `/admin/safe-houses` | rhc_admin |  |
| `POST` | `/admin/safe-houses/{id}` | rhc_admin |  |
| `GET` | `/admin/audit` | rhc_admin |  |
| `GET` | `/occupants` | login | house scope (`housesForUser`) |
| `GET` | `/occupants/new` | login |  |
| `POST` | `/occupants` | login | house scope of the submitted house; form validation |
| `GET` | `/occupants/{id}/edit` | login | house scope (`canAccessHouse`) |
| `POST` | `/occupants/{id}/rename` | login | house scope; nickname unique per house |
| `POST` | `/occupants/{id}/demographics` | login | house scope |
| `POST` | `/occupants/{id}/depart` | login | house scope |
| `GET` | `/expenses` | login | house scope |
| `GET` | `/expenses/new` | login |  |
| `POST` | `/expenses` | login | house scope of the submitted house; form validation |
| `GET` | `/expenses/{id}/edit` | login | `loadExpenseInScope` |
| `POST` | `/expenses/{id}` | login | `loadExpenseInScope` |
| `POST` | `/expenses/{id}/review` | login | `canReviewExpenses` (rhl_admin, rhc_admin) + scope |
| `GET` | `/expenses/{id}/receipt` | login | `loadExpenseInScope`; 404 without receipt |
| `POST` | `/expenses/{id}/delete` | login | `loadExpenseInScope` |
| `GET` | `/operations` | login | house scope |
| `GET` | `/operations/new` | login |  |
| `POST` | `/operations` | login | house scope of the submitted house; form validation |
| `GET` | `/operations/{id}/edit` | login | house scope |
| `POST` | `/operations/{id}` | login | house scope |
| `GET` | `/reports` | login | `?house=` checked with `canAccessHouse`; future month redirects |
| `GET` | `/reports/{houseID}/{file}` | login | `canAccessHouse`; `{file}` must end in `.csv`; future month 400 |
| `GET` | `/reports/{houseID}/history` | login | house scope |
| `POST` | `/reports/{houseID}/{month}/confirm` | login | `canConfirmMonth` (manager, rhl_admin; not rhc_admin); month ended; fingerprint |
| `GET` | `/dashboard` | login | rhl_admin and rhc_admin only |
| `GET` | `/safeguarding` | login | `canAccessSafeguarding` (= house scope) |
| `GET` | `/safeguarding/new` | login |  |
| `GET` | `/safeguarding/document-37` | login | logged in; docx download |
| `POST` | `/safeguarding` | login | house scope of the submitted house; form validation |
| `GET` | `/safeguarding/{id}/edit` | login | `canAccessSafeguarding` |
| `POST` | `/safeguarding/{id}` | login | `canAccessSafeguarding` |

Note: `GET /reports/{houseID}/history` wins over `GET /reports/{houseID}/{file}` because Go's mux prefers the more specific pattern; the matrix probes both.
