# AskDesk

> A multi-channel AI customer-support layer. One reply engine, many channels.

AskDesk answers customer questions from a business's own FAQ knowledge base using
retrieval-augmented generation (RAG). Customers reach it over **Telegram** or a
**JSON API** you embed in any web app (e.g. [minipos](https://minipos.site)). It's
built to be **self-hostable** and multi-tenant-ready — serving another business is
data, not code.

Positioned as a **low-cost, self-hostable alternative** to SaaS support tools like
Intercom and Chatbase: free-tier AI, cost-ordered provider failover, and
self-hosting are the cost advantage, not an afterthought.

---

## Features

- **Two channels, one brain** — a Telegram bot and a JSON API share the same
  engine, FAQs, and admin tools.
- **Buttons + AI** — customers tap *category → question* for instant answers
  (zero AI cost); free-typed questions go to RAG + AI.
- **Human handoff** — when the AI is unsure, the question is queued and the admin
  gets a button panel (`/admin`): view pending questions and **tap-to-reply** —
  the bot relays your answer straight to the customer.
- **Provider failover** — a cost-ordered AI chain with a circuit breaker; falls
  over to the next provider on quota limits or outages.
- **Runtime config** — shop name, welcome/fallback messages, and FAQs are edited
  from your phone via a signed magic-link web form. No redeploy.
- **Two deploy modes** — all-in-one (one free process) or web + worker split
  (Redis), chosen by a single env var.

---

## Architecture

```
Telegram bot ─┐
              ├─→ normalize → { businessId, channel, userId, text }
Web/JSON API ─┘                       │
                                      ▼
                       CORE REPLY ENGINE  (channel-agnostic)
                         1. RAG lookup  — pgvector similarity search over FAQs
                         2. Confidence  — high → answer; low → fallback + queue
                         3. Generate    — AI provider chain (failover + breaker)
                         4. Log         — conversations; flag unanswered
                                      │
                         reply returned on the same channel
```

Adding a channel is a small adapter that produces `core.Message` — the engine
never changes. Every table carries `business_id`, so a second business is a new
row, not a migration.

---

## AI provider failover

```
generateReply(msg)
   → Provider #1 (cheapest/free)  → ok? return
   → quota / error / timeout      → Provider #2 → … → all exhausted → fallback + queue
```

A circuit breaker skips a repeatedly failing provider for a cooldown.
**Generation** chains freely across providers; **embeddings use one fixed
provider** (vectors from different models can't share a similarity index).

---

## Channels

**Telegram** — button menu built from FAQ categories, free-text → AI, admin panel
with tap-to-reply, and a magic-link FAQ/settings editor. Webhook verified by a
secret token.

**Web / JSON API** — `/api/v1`, authenticated by an `X-API-Key` header:

| Endpoint | Returns |
|---|---|
| `GET /api/v1/config` | shop name, welcome text, categories |
| `GET /api/v1/faqs` | categories with questions + answers |
| `POST /api/v1/ask` | `{ answer, answered }` (free text → AI) |

Read-only and tenant-isolated; an empty knowledge base returns empty JSON.

---

## Security

- **Telegram webhook** secret token verified in constant time
- **Tenant isolation** — `business_id` scoping on every query (covered by tests)
- **Admin auth** — signed, short-lived magic links → signed HttpOnly session
  (no passwords in chat)
- **Parameterized queries** (pgx); **prompt-injection-aware** prompt (FAQ text is
  data, the AI is read-only)
- **Web API** is read-only, API-key-authenticated, CORS-allowlisted
- **CI**: `go vet`, race tests, build, `govulncheck` + Dependabot; distroless
  non-root container image

---

## Data model

All tables scoped by `business_id`.

| Table | Purpose |
|---|---|
| `businesses` | Tenant: name, API key, JSONB settings (name, messages) |
| `faqs` | Knowledge base: question, answer, `embedding vector(768)`, category |
| `conversations` | Log: question, matched FAQ, AI answer, confidence, answered |
| `unanswered_queue` | Questions pending an admin answer |
| `admins` | Identity allow-list: business, channel, external id |

---

## Tech stack

| Layer | Choice |
|---|---|
| Language | **Go** |
| Database | **PostgreSQL + pgvector** |
| AI | **Gemini** (generation + embeddings) behind a cost-ordered provider chain |
| Queue (optional) | **Redis + asynq** (web + worker split) |
| Migrations | **golang-migrate**, embedded in the binary (auto-applied) |
| Channels | **Telegram**, **Web/JSON API** |
| Container | **Docker** (distroless, non-root) |
| CI | **GitHub Actions** (vet, race tests, build, govulncheck) + Dependabot |

---

## Deploy

Same code, two shapes — chosen by whether `ASKDESK_REDIS_URL` is set:

- **All-in-one** (no Redis) — one process runs everything. Free tier.
- **Web + worker** (Redis) — thin web tier enqueues; a `worker` runs the engine.
  Paid / always-on.

**Quick start:** create a Telegram bot + a Postgres DB (Supabase) + a Gemini key,
deploy the Docker image (Render or your own server) with those as env vars, seed
an admin, load your FAQs, and register the webhook.

→ Full step-by-step (free **and** paid), plus the Web API guide: **[DEPLOY.md](DEPLOY.md)**

---

## Status

Working and deployed (Render + Supabase, free tier): Telegram bot (button menu,
admin panel, magic-link editor), the Web/JSON API, RAG, provider failover, and
runtime settings — running for [minipos](https://minipos.site).

**Roadmap:** WhatsApp / Messenger adapters · embeddable web chat widget · per-user
rate limiting · multi-tenant self-service onboarding · observability.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).
