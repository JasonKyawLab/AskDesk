<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="internal/widget/askdesk-logo-white.svg">
    <img alt="AskDesk" src="internal/widget/askdesk-logo.svg" width="300">
  </picture>
</p>

<p align="center">
  <b>Every channel. One desk.</b><br>
  A self-hostable, multi-channel AI customer-support layer.
</p>

---

AskDesk answers customer questions from your own FAQ knowledge base using
retrieval-augmented generation (RAG) — across **Telegram**, **Facebook
Messenger**, an **embeddable web widget**, and a **JSON API** — and hands off to
a human whenever the AI isn't confident. One engine, many channels, your data.

A low-cost, self-hostable alternative to SaaS tools like Intercom and Chatbase.

## Features

- **Many channels, one brain** — Telegram, Messenger, a drop-in web widget, and
  a JSON API share the same engine, FAQs, admin, and inbox.
- **Answers only when sure** — a confident FAQ match gets an AI answer; anything
  weaker skips the AI (no guessing, no wasted tokens) and hands off to a human.
  Prefer no AI at all? Run **FAQ-only mode**.
- **Embeddable widget** — one `<script>` line adds the chat bubble to any site,
  with an optional **contact-gate** that turns support into lead capture.
- **Multiple languages** — offer FAQs in any set of languages (e.g. English,
  Burmese, Chinese). Each is authored natively — no machine translation — and the
  widget shows a language switcher; the AI answers in the chosen language.
- **Cross-channel handoff** — unanswered questions land in one shared inbox with
  the sender's channel and name. Reply from Telegram, a web page, or your own
  app; the answer routes back to the customer's channel.
- **Provider failover + rate limiting** — a cost-ordered AI chain with a circuit
  breaker, plus adjustable per-user and whole-deployment limits.
- **Runtime config** — shop name, messages, rate limits, data retention, and
  FAQs are edited from your phone via a signed magic link. No redeploy.
- **Self-hostable & multi-tenant** — every row scoped by `business_id`; runs on a
  free tier (Render + Supabase) or your own server.

## Channels & API

**Telegram · Messenger · Web widget** — all button-menu + AI, sharing one inbox.

**JSON API** (`/api/v1`, `X-API-Key`): `config` · `faqs` · `ask` · `replies` ·
`lead`. A separate **admin API** (`/api/v1/admin`, `X-Admin-Key`) lets your own
app read the queue and reply, pull leads + analytics, and **manage FAQs**
(create/update/delete) — so you can build FAQ editing into your own admin instead
of the magic-link page. Embed the widget with:

```html
<script src="https://<your-host>/widget.js" data-key="<public api key>"></script>
```

## Security

Signed magic-link admin auth (no passwords in chat) · webhook verification
(Telegram secret token, Messenger `X-Hub-Signature-256`) · `business_id` tenant
isolation on every query · parameterized queries · read-only, injection-aware
prompts · CI with `go vet`, race tests, and `govulncheck` · distroless container.

## Deploy

Same code, two shapes — chosen by a single env var:

- **All-in-one** (no Redis) — one process. Free tier.
- **Web + worker** (Redis) — thin web tier enqueues; a `worker` runs the engine.

→ Full step-by-step (free **and** paid), channels, and the widget: **[DEPLOY.md](DEPLOY.md)**

## Tech

**Go** · **PostgreSQL + pgvector** · **Gemini** (behind a provider chain) ·
**golang-migrate** (embedded) · optional **Redis + asynq** · **Docker**
(distroless) · **GitHub Actions** CI.

## License

**AGPL-3.0** — see [LICENSE](LICENSE). Running a modified version as a network
service means sharing your source (AGPL §13). **Commercial licenses available on
request.**
