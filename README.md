# AskDesk

> A reusable, multi-channel AI customer support layer. One shared reply engine, many channels.

AskDesk lets customers ask questions over **Telegram, WhatsApp, Messenger, or a web widget**, and answers them from a business's own FAQ knowledge base using retrieval-augmented generation (RAG). Admins manage the knowledge base and read stats through the **same channels their customers already use** — no separate app required.

It is built first as a feature for [minipos](https://minipos.site), but designed from day one to be reusable by other businesses through configuration rather than code changes.

---

## Why AskDesk

- **Channel-agnostic core.** Every channel is a thin adapter over a single reply engine. The AI/RAG logic exists once, not duplicated per channel.
- **Multi-tenant from day one.** Every table carries a `business_id`, so serving another business is a config change — not a schema migration.
- **Provider-abstracted AI.** Swap the language model (Gemini → OpenAI) through configuration; the engine never changes.
- **No passwords in chat.** Admin access is identity-based, verified against an `admins` table — never a typed password sitting in chat history.
- **Graceful degradation.** When the daily AI quota is hit, messages are queued and the admin is notified rather than showing customers a broken bot.

---

## How it works

Customer messages from any channel are normalized into a single shape and funneled into one engine:

```
      Telegram webhook   ──┐
Messenger / WhatsApp API  ──┼──▶ normalize ──▶ { businessId, channel, userId, message }
      Web widget API     ──┘                                │
                                                            ▼
                                        CORE REPLY ENGINE (shared, channel-agnostic)
                                          1. RAG lookup   — pgvector similarity search over FAQs
                                          2. Confidence   — high → answer from FAQ context
                                                            low  → answer + flag to unanswered queue
                                          3. Generate     — AIProvider.generateReply()
                                                              ├── GeminiProvider (free, default)
                                                              └── OpenAIProvider (paid, config swap)
                                          4. Log          — write to conversations table
                                                            │
                                                            ▼
                                        Reply is sent back through the same channel it arrived on
```

Admin messages follow their own path, funneling into an Admin API after an identity check:

```
Telegram admin command ──┐
WhatsApp admin message ──┼──▶ identity check vs. admins table ──▶ Admin API
minipos admin dashboard──┘                                          /api/admin/faqs        add / edit / delete FAQ
                                                                    /api/admin/stats       today's volume, answered/unanswered
                                                                    /api/admin/unanswered  pending queue
```

**Two APIs, one engine.** Customer entry points all funnel into `generateCustomerReply()`. Admin entry points all funnel into the Admin API. Neither duplicates logic per channel — adding a new channel means writing a small in/out adapter, nothing more.

---

## Admin authentication (no passwords in chat)

Passwords typed into a chat sit permanently in history, are phishable via bot impersonation, and aren't fully private on WhatsApp (routed through Meta/BSP infrastructure). AskDesk avoids them entirely:

1. **Identity as auth.** The owner's Telegram `user_id` or verified WhatsApp number is added to the `admins` table once during setup. Every admin command checks the sender's identity against that table.
2. **Two interaction modes.**
   - *Quick read-only actions* (`/stats`, `/unanswered`) run directly in chat — low risk.
   - *Content editing* (FAQ add/edit) opens through a **signed, short-lived magic link** ("here's your edit link, expires in 10 minutes") to a lightweight mobile web form. Possession of the messaging account already proved identity, so no separate login is needed.
3. **Session timeout** (~15 min) on the magic-link session limits exposure if a device is later compromised.

---

## Data model

All tables are scoped by `business_id` to keep the system multi-tenant.

| Table | Purpose |
|---|---|
| `businesses` | Tenant record: name, API key, channel credentials |
| `faqs` | Knowledge base: question, answer, `embedding vector(768)`, category |
| `conversations` | Full log: question, matched FAQ, AI answer, confidence, answered flag |
| `unanswered_queue` | Low-confidence questions pending an admin answer |
| `admins` | Identity allow-list: business, channel, external id (Telegram/WhatsApp/minipos) |

<details>
<summary>SQL sketch</summary>

```sql
businesses (
  id, name, api_key, telegram_bot_token, whatsapp_number, created_at
)

faqs (
  id, business_id, question, answer, embedding vector(768), category, updated_at
)

conversations (
  id, business_id, channel, external_user_id, question,
  matched_faq_id, ai_answer, confidence_score, was_answered boolean, created_at
)

unanswered_queue (
  id, conversation_id, question,
  status text default 'pending',  -- pending | resolved
  created_at
)

admins (
  business_id, channel, external_id, name
  -- external_id = Telegram user_id, WhatsApp number, or minipos user id
)
```
</details>

---

## Architecture principles

- **Separate service, separate repo.** AskDesk runs as its own process/container with an independent deploy pipeline. A crash, memory spike, or runaway AI loop in AskDesk must never be able to take down POS terminals mid-transaction.
- **Shared infrastructure now, isolated later.** Initially both minipos and AskDesk run as separate Docker containers on the same host, sharing one Postgres instance (different tables). Once real paying customers depend on uptime, AskDesk moves to its own server — an infra change, not a code change, because it was always a separate service.

---

## Tech stack

| Layer | Choice |
|---|---|
| Runtime | Node.js |
| Database | PostgreSQL + `pgvector` |
| AI (default) | Google Gemini Flash (free tier) |
| AI (swap) | OpenAI (config only) |
| Queue | Redis + BullMQ |
| Channels | Telegram, WhatsApp, Messenger, web widget |
| Deploy | Docker |

---

## Roadmap

- [ ] Core reply engine — RAG lookup, confidence check, provider abstraction
- [ ] Telegram channel adapter (primary demo channel — fully free, no cap)
- [ ] Web widget for embedding on minipos.site
- [ ] Admin API + magic-link FAQ editor
- [ ] Redis/BullMQ queueing and graceful degradation on quota limits
- [ ] Observability and automated test coverage
- [ ] WhatsApp channel (Phase 2 — requires a BSP; enable once there's a paying user)

---

## Status

Early development. Architecture and data model are defined; implementation is in progress.
