# Sandesh Nagari 7 — plot society portal

<p>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Postgres" src="https://img.shields.io/badge/Postgres-16-4169E1?style=flat-square&logo=postgresql&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=black">
  <img alt="Hosting" src="https://img.shields.io/badge/hosting-free%20tier-57A55B?style=flat-square">
</p>

Most people who buy a plot never live near it. They buy in Nagpur and live in
Pune, Bengaluru or Dubai, and every question they have — is my plot still clear,
where did the maintenance money go, when does the road get tarred — becomes a
phone call to the builder's sales desk. That desk is the bottleneck, and it is
what this replaces.

Built against **Sandesh Nagari 7** by Shiv Rudra Group: 823 plots across 58
NMRDA-sanctioned acres at Rui &amp; Banwadi on the Wardha Road–MIHAN corridor,
Nagpur. Plot numbers, sector ranges, areas and the RERA number come from the
sanctioned layout; ownership, dues, ledger figures and queries are demo data.

---

## Three levels of access

| | Who | Sees | Signs in? |
|---|---|---|---|
| **Guest** | Anyone with the link | Layout map, what is available and at what price, amenities, recent site progress — and can leave an enquiry | **No account** |
| **Owner** | A registered plot owner | Everything a guest sees, plus their plot, dues, documents, the full society fund ledger, and their own query threads | Yes |
| **Builder** | Site office and staff | Everything, plus the admin portal: query inbox with SLA clocks, leads, plot status, progress posts, ledger entry | Yes |

A guest is not a user row. "Guest" is the absence of a token on a small set of
read-only routes plus one rate-limited write, the enquiry — which keeps
authorisation to three real states instead of a fourth identity to maintain.

Every guest enquiry lands in the builder's **Leads** board with the plot they
were looking at, their number, and a one-tap call and WhatsApp link. That is the
commercial reason for a builder to share the public link at all.

---

## What each side actually gets

**Owners**
- Live layout map of all 823 plots, colour-coded by sector and status
- Their plot: area, facing, corner, documents, maintenance dues and receipts
- Dated site-progress feed with photos
- The society fund ledger — collected, spent by head, closing balance
- Queries with a **visible SLA clock**, in eight categories

**Builders**
- One dashboard: what is past its deadline, what is open, new leads, fund balance
- Query inbox that sorts breached SLAs to the top, with internal notes owners cannot see
- Leads board with call / WhatsApp and a status pipeline
- Inline plot status editing across all 823 plots
- Post a progress update with photos, publish or save as a draft
- Add ledger entries; corrections are reversals, never edits

---

## Stack

| Layer | Choice | Why |
|---|---|---|
| API | Go 1.23, standard library router | One static binary, ~15 MB image, milliseconds per request |
| Database | Postgres 16 | Plots ↔ owners ↔ payments ↔ queries is relational |
| Object storage | S3-compatible — R2 in production, MinIO locally | Photos never touch the API or the database |
| Frontend | Vite + React 18 + TypeScript + Tailwind | |
| Auth | Argon2id + JWT access tokens + single-use refresh tokens | No third-party auth, no per-user cost |

Four direct Go dependencies: `pgx`, `golang-jwt`, `uuid`, `x/crypto`. S3 request
signing is written against the standard library rather than pulling in the AWS
SDK — about 100 lines, and it removes three dependencies.

**Cost to run: ₹0/month.** Cloud Run's always-free tier for the API, Supabase
Postgres, Cloudflare R2 for photos, Cloudflare Pages for the front end. Go is
what makes that fit: Cloud Run bills CPU-seconds, and a Go handler returns in
single-digit milliseconds. See [`docs/DEPLOY.md`](docs/DEPLOY.md).

---

## Running it locally

Needs Docker and Go 1.23 (`brew install go`).

```bash
make up      # Postgres + MinIO
make seed    # Sandesh Nagari 7: 823 plots, ledger, progress posts, queries, leads
make run     # API on :8080

cd web && npm install && npm run dev   # UI on :5173
```

`make seed` prints the demo logins:

| Role | Email | Password |
|---|---|---|
| Builder admin | `office@shivrudragroup.in` | `sandesh-demo-2026` |
| Site staff | `sitedesk@shivrudragroup.in` | `sandesh-demo-2026` |
| Plot owner | `rohit.deshmukh@example.com` | `sandesh-demo-2026` |
| Guest | — no sign-in — | visit `/explore` |

Change that password before showing this to anyone outside your laptop.

---

## Layout

```
cmd/api/            server entry point
cmd/seed/           the Sandesh Nagari 7 layout, for a sales demo
internal/
  config/           env loading; fails at boot, never mid-request
  database/         pgx pool + embedded migrations
  httpx/            typed errors, JSON, middleware
  domain/           roles, plot statuses, the SLA table
  auth/             Argon2id, JWT, invites, role gates
  plot/             the layout map and plot detail
  query/            owner tickets, threads, SLA clocks
  fund/             append-only society ledger
  update/           construction progress feed
  media/            presigned uploads (SigV4, no SDK)
  lead/             guest access and enquiries
test/               schema smoke tests
web/                React app — owner views and the builder portal
```

---

## Three decisions worth knowing about

**The fund ledger is append-only.** A correction writes a reversal row linked to
the original; nothing is updated or deleted. When an owner disputes a figure the
builder can show the whole history, rather than a number that quietly changed.
That is the difference between a ledger and a spreadsheet.

**The layout map carries no owner details.** Every signed-in owner sees which
plots are sold, available or on hold — that is the point of the map — but names
and numbers appear only on a plot's own detail page, to that plot's owner and to
the builder. An owner directory exists behind an explicit opt-in.

**Photos never pass through the API.** The browser gets a 15-minute presigned
URL and uploads straight to object storage. A scale-to-zero container cannot
afford to stream image bytes, and this is what keeps it inside a free CPU
allowance.

---

## Verification status

- ✅ Schema applies cleanly; 9 schema/business-rule smoke tests pass (`make smoke`)
- ⏳ Go build and tests not yet run — the toolchain is not installed on this machine
- ⏳ Frontend `npm install` / typecheck not yet run

Run `brew install go && make build && make test` and `cd web && npm install &&
npm run typecheck` to close the last two.
