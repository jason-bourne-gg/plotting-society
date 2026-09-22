# Deploying

Two paths. Both run the same container image, so moving between them is an
environment-variable change, not a migration.

---

## Path A — free forever, no card anywhere

Best for the demo you show a builder before anyone has signed anything.

| Piece | Service | Free allowance | Card needed |
|---|---|---|---|
| Database | Supabase Postgres | 500 MB | No |
| File storage | Supabase Storage | 1 GB | No |
| API | Render web service | 750 hours/month | No |
| Frontend | Cloudflare Pages | Unlimited | No |
| Email | Resend | 3,000/month | No |

The cost is cold starts: Render spins an idle free service down, so the first
request after a quiet period takes 20–50 seconds. Fine for a demo you are
driving, wrong for owners opening the app on their own.

## Path B — free forever, sub-second, needs a card on file

| Piece | Service | Free allowance | Card needed |
|---|---|---|---|
| Database | Supabase Postgres | 500 MB | No |
| API | Google Cloud Run | 2M requests + 180k vCPU-sec/month | Yes |
| Photos | Cloudflare R2 | 10 GB, zero egress | Yes |
| Frontend | Cloudflare Pages | Unlimited | No |
| Email | Resend | 3,000/month | No |

Cloud Run bills by CPU-second. A Go handler returns in single-digit
milliseconds, so 1,000 users generate roughly 600k requests and 30k vCPU-seconds
a month — comfortably inside an allowance that never expires. Cold start is
~400 ms for the container plus ~500 ms if Postgres has suspended.

Set a budget alert at ₹100 on both accounts. You should never see a bill; the
alert is there so you find out immediately if that changes.

---

## Path B, step by step

### 1. Database

Create a Supabase project, then take **Settings → Database → Connection string
→ URI**. Use the **pooler** URI (port 6543), not the direct one — Cloud Run
opens and drops connections as it scales, and the pooler is what absorbs that.

```
DATABASE_URL=postgres://postgres.xxxx:PASSWORD@aws-0-ap-south-1.pooler.supabase.com:6543/postgres
```

Migrations apply themselves when the container boots. There is no separate
migration step to run or forget.

### 2. Object storage

Create an R2 bucket named `plotting-media`, then an API token scoped to it.
Enable public read access on the bucket (photos are meant to be seen; the
secret is the URL, and keys contain a UUID).

```
S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET=plotting-media
S3_ACCESS_KEY_ID=...
S3_SECRET_ACCESS_KEY=...
S3_PUBLIC_BASE_URL=https://pub-<hash>.r2.dev
```

### 3. API

```bash
gcloud auth login
gcloud config set project <your-project>
gcloud run deploy plotting-api \
  --source . \
  --region asia-south1 \
  --allow-unauthenticated \
  --min-instances 0 \
  --max-instances 3 \
  --memory 256Mi \
  --set-env-vars "APP_ENV=production,PUBLIC_BASE_URL=https://app.yourdomain.in,CORS_ORIGINS=https://app.yourdomain.in" \
  --set-secrets "DATABASE_URL=database-url:latest,JWT_SECRET=jwt-secret:latest,S3_SECRET_ACCESS_KEY=r2-secret:latest"
```

`--max-instances 3` is the guard rail that matters. Without it, a traffic spike
or a crawl loop can scale you out of the free tier and into a real bill.

Generate the JWT secret once and put it in Secret Manager, never in a env var
you can read off a dashboard:

```bash
openssl rand -base64 48 | gcloud secrets create jwt-secret --data-file=-
```

### 4. Seed the demo society

```bash
gcloud run jobs create seed-demo \
  --image <same-image> --command /seed \
  --region asia-south1 \
  --set-secrets "DATABASE_URL=database-url:latest"
gcloud run jobs execute seed-demo
```

### 5. Frontend

Cloudflare Pages, pointed at `web/`, with `VITE_API_BASE_URL` set to the Cloud
Run URL. Add your domain there and the TLS certificate is automatic.

---

## When a builder signs

Nothing is rewritten. Either:

- **Stay serverless** — move Supabase to Pro ($25/month) for a dedicated
  instance and daily backups, or
- **Move to one box** — an AWS Lightsail instance in Mumbai at $5/month, fixed
  price, running the same image with Postgres alongside it. Use this when a
  builder wants to be told their data sits on their own server.

The pitch stays honest either way: charge per society per month, and the
infrastructure is a rounding error against it.

---

## What to check before showing anyone

- [ ] `JWT_SECRET` is 48 random bytes, not the example value
- [ ] `CORS_ORIGINS` names your real front-end domain only
- [ ] `--max-instances` is set
- [ ] Budget alerts are on both Google Cloud and Cloudflare
- [ ] The seeded demo password has been changed
- [ ] `/healthz` returns 200 from outside your network
