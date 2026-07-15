# AskDesk

> A multi-channel AI customer support layer. One reply engine, many channels.

AskDesk answers customer questions from a business's own FAQ knowledge base using
retrieval-augmented generation (RAG). Customers reach it over **Telegram** (WhatsApp,
Messenger, and an embeddable web widget later). Built first for
[minipos](https://minipos.site), and architected so serving another business is a config
change, not a rewrite.

It's positioned as a **low-cost, self-hostable alternative** to SaaS support tools like
Intercom and Chatbase — aimed at small businesses priced out of per-seat subscriptions.
Free-tier AI, cost-ordered provider failover, and self-hosting are the cost advantage,
not an afterthought.

---

## Key decisions

- **Channel-agnostic core.** Channels are thin adapters over one reply engine — the AI/RAG
  logic exists once, not per channel.
- **AI provider failover.** A cost-ordered chain of providers: cheapest/free first, falling
  over to the next on quota limits or outages. Reordering or swapping is config, not code.
- **Multi-tenant–ready.** Every table carries `business_id`, so a second business is a new
  row, not a migration.
- **Web / worker split.** The web tier answers webhooks in milliseconds and enqueues the
  slow AI call; workers process the queue with retries and graceful degradation.
- **Secure by default.** Verified webhooks, tenant isolation, encrypted tokens, and rate
  limiting from the first commit.

---

## Architecture

```
   Telegram / widget / Meta webhooks
                │  normalize → { businessId, channel, userId, message }
                ▼
        CORE REPLY ENGINE
          1. RAG lookup    — pgvector similarity search over FAQs
          2. Confidence    — high → answer from context; low → answer + flag to queue
          3. Generate      — AI provider chain (failover)
          4. Log           — conversations table
                │  (slow AI work runs on a background worker via the queue)
                ▼
        Reply sent back through the same channel it arrived on
```

Admin messages take a separate path: an identity check against the `admins` table, then
the Admin API (`/faqs`, `/stats`, `/unanswered`). Read-only actions (`/stats`) run in
chat; FAQ editing opens via a signed, short-lived magic link — no passwords in chat.

---

## AI provider failover

```
generateReply(msg)
   → Provider #1 (cheapest/free)  → ok? return
   → quota / error / timeout      → Provider #2 → … → all exhausted → queue + notify admin
```

A circuit breaker skips a repeatedly failing provider for a cooldown.

**Generation vs. embeddings:** generation is chained freely across providers. **Embeddings
use one fixed provider** — vectors from different models have different dimensions and
can't share a similarity index. Switching it means re-embedding all FAQs (a migration).

---

## Security

- **Verified webhooks** — Telegram secret-token / Meta HMAC checked on every request
- **Tenant isolation** — `business_id` scoping enforced centrally on every query
- **Encrypted channel tokens** at rest; **parameterized queries** (pgx); least-privilege DB
- **Rate limiting** per user and per business (Redis) — abuse + cost control
- **Prompt-injection aware** — FAQ text treated as data; the AI is read-only
- **CI scanning** — `govulncheck`, `gosec`, secret scanning; hardened non-root containers

---

## Data model

All tables scoped by `business_id`.

| Table | Purpose |
|---|---|
| `businesses` | Tenant: name, API key, encrypted channel credentials |
| `faqs` | Knowledge base: question, answer, `embedding vector(768)`, category |
| `conversations` | Log: question, matched FAQ, AI answer, confidence, answered flag |
| `unanswered_queue` | Low-confidence questions pending an admin answer |
| `admins` | Identity allow-list: business, channel, external id |

---

## Tech stack

| Layer | Choice |
|---|---|
| Language | **Go** |
| Database | **PostgreSQL + pgvector** |
| Cache / queue | **Redis + asynq** |
| AI | **Provider chain** (Gemini Flash default; others as fallback) |
| Channels | **Telegram** first; WhatsApp / Messenger / web widget later |
| Deploy | **Docker**, **Terraform**, **GitHub Actions** |
| Observability | **Prometheus + Grafana + OpenTelemetry** |
| Testing | **Go test + testcontainers** |

---

## Roadmap

**Phase 1 — single-tenant bot (free tier)**
Go backend (web + worker) · core reply engine · AI provider chain with failover · Telegram
adapter · security non-negotiables · Redis/asynq queue · magic-link FAQ editor ·
integration tests + CI · deploy to OCI for real minipos questions.

**Phase 2 — multi-tenant product**
Self-service onboarding/admin frontend · embeddable web chat widget · observability stack ·
Terraform-provisioned infra.

**Phase 3 — scale (multi-tenant hosting)**
WhatsApp (needs a BSP) · dedicated host · **Kubernetes (k3s)** — only once hosting many
businesses: independent web/worker scaling, autoscaling the worker pool on queue depth
(HPA), and zero-downtime rollouts. Docker Compose remains correct until then.

---

## Status

Early development — architecture, data model, and security model defined; Phase 1 in
progress.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).
