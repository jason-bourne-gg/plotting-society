# Going live — free, no credit card

Three accounts, none of which asks for a card. Roughly 40 minutes end to end.

| Piece | Service | Free tier |
|---|---|---|
| Database + photos | Supabase | 500 MB Postgres, 1 GB files |
| API | Render | 750 instance-hours/month, Docker |
| Frontend | Cloudflare Pages | unlimited bandwidth |
| Invite emails *(optional)* | Resend | 3,000/month |

Everything here permits commercial use. Vercel's free Hobby plan does not, which
is the one reason this guide does not use it.

---

## 1. Supabase — the database

1. **supabase.com** → new project. Region **Mumbai (ap-south-1)**. Save the
   database password somewhere; it is shown once.
2. Settings → Database → Connection string → **URI**.
3. Take the **pooler** URI, the one on port **6543**, not the direct 5432 one.
   Render restarts a free service whenever it idles, and a free Postgres has a
   low connection cap; the pooler is what absorbs that.

```
postgres://postgres.abcdefgh:YOUR-PASSWORD@aws-0-ap-south-1.pooler.supabase.com:6543/postgres
```

That string is `DATABASE_URL`. **The migrations run themselves** when the API
boots — there is no separate step to run or forget.

### Photo storage (can wait)

Storage → new bucket `plotting-media`, **public**. Then Storage → S3 access keys
→ new key. Supabase Storage speaks the S3 API, so the signer in
`internal/media` works against it with no code change:

```
S3_ENDPOINT=https://abcdefgh.supabase.co/storage/v1/s3
S3_REGION=ap-south-1
S3_BUCKET=plotting-media
S3_ACCESS_KEY_ID=...
S3_SECRET_ACCESS_KEY=...
S3_PUBLIC_BASE_URL=https://abcdefgh.supabase.co/storage/v1/object/public/plotting-media
```

Leave these blank for now if you like: the upload route returns a clear 503
saying uploads are not set up, and everything else works.

---

## 2. Render — the API

1. **render.com** → sign in with GitHub, authorise `jason-bourne-gg`.
2. **New → Blueprint** → pick `plotting-society`. Render reads `render.yaml`,
   finds the Dockerfile and creates the service.
3. It will ask for the values marked `sync: false`. Set `DATABASE_URL` now and
   leave the rest blank — you do not have the Pages URL yet.
4. First build takes 3–5 minutes. When it is green, check:

```bash
curl https://plotting-society-api.onrender.com/healthz
# {"status":"ok"}
```

A 200 here means the container booted, reached Supabase **and applied both
migrations**. If it fails, Logs will say which.

### Seed the demo data

From your laptop, pointed at the live database:

```bash
cd ~/Desktop/Code/plotting-society
DATABASE_URL='<the pooler URI>' go run ./cmd/seed
```

That loads Sandesh Nagari 7 — 823 plots, the ledger, progress posts, queries and
guest leads — and prints the logins.

---

## 3. Cloudflare Pages — the frontend

1. **dash.cloudflare.com** → Workers & Pages → Create → Pages → Connect to Git →
   `plotting-society`.
2. Build settings:

| Field | Value |
|---|---|
| Framework preset | Vite |
| Build command | `npm run build` |
| Build output directory | `dist` |
| Root directory | `web` |

3. Environment variables → add:

```
VITE_API_BASE_URL = https://plotting-society-api.onrender.com
```

This is read at **build time**, so changing it later needs a redeploy, not just
a save.

4. Deploy. You get `https://plotting-society.pages.dev`.

`web/public/_redirects` sends every unmatched path to `index.html`, so
`/my-plot` survives a refresh instead of 404ing.

---

## 4. Close the loop

Back in Render → Environment, now that you have the Pages URL:

```
PUBLIC_BASE_URL = https://plotting-society.pages.dev
CORS_ORIGINS    = https://plotting-society.pages.dev
```

Save; Render restarts. **Until this is set the browser will block every API
call** — that is CORS doing its job, not a bug. If the UI loads but nothing
fetches, this is almost always why: check the browser console for a CORS error
and confirm the origin matches exactly, with no trailing slash.

---

## 5. Kill the cold starts

Render idles a free service down after 15 minutes, so the first request after a
quiet spell takes 20–50 seconds. But 750 free instance-hours covers a 744-hour
month, so keeping it awake permanently fits inside the free tier.

**uptimerobot.com** (also no card) → new HTTP(s) monitor →
`https://plotting-society-api.onrender.com/healthz`, every 10 minutes.

`/healthz` pings the database, so this also stops Supabase pausing the project
after 7 days of no traffic.

---

## Before you show anyone

- [ ] `/healthz` returns 200 from your phone, off wifi
- [ ] The guest view at `/explore` loads and the enquiry form submits
- [ ] Sign in as the builder admin and as an owner
- [ ] **Change the seeded password** — `sandesh-demo-2026` is in a public repo
- [ ] Post one real site photo, so the progress feed is not obviously fake
- [ ] Uptime monitor is green

---

## When a builder signs

Nothing gets rewritten. Either:

- **Stay serverless** — Render at $7/month removes the idling and the ping;
  Supabase Pro at $25 adds a dedicated instance and daily backups.
- **One box** — AWS Lightsail Mumbai, $5/month fixed, running the same image
  with Postgres beside it. Use this when a builder wants to be told their data
  sits on their own server.

Charge per society per month and the infrastructure is a rounding error.
