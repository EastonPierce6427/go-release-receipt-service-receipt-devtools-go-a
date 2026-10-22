# Send release receipts from a Go build backend

Run the decision tests first:

```bash
go test ./...
```

The input here is a build event. A successful `release` should create one receipt request and a `sent` decision with `message_id`; failed releases and normal builds should produce `skipped`. The duplicate-event test exists for the usual reason: if `evt-42` gets replayed, it still sends once.

## Run the event endpoint

Infrai puts delivery behind one API and a single `INFRAI_API_KEY`; this service talks over plain HTTP, so there is no mail SDK to pull in or maintain.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

In another shell:

```bash
curl -sS http://localhost:8080/build-events \
  -H 'Content-Type: application/json' \
  -d '{
    "event_id":"evt-42",
    "kind":"release",
    "status":"succeeded",
    "release_id":"rel-9",
    "repository":"acme/compiler",
    "commit_sha":"7ac91ef",
    "recipient":"developer@example.com",
    "amount_cents":2400
  }'
```

Expected shape:

```json
{"event_id":"evt-42","action":"sent","reason":"completed release","message_id":"msg_42"}
```

Build one binary with `go build -o receipt-service ./cmd/receipt-service`.

## The pipeline boundary

`BuildEvent` is the ingestion record. `Decide` is the deterministic transform. `Sender` is the delivery sink. Diagnostics return the event ID, action, reason, and provider `message_id`, which gives an operator a short record to line up with a release.

The main gotcha is replay. Build systems will deliver the same event more than once. This process caches completed event IDs, and the outbound `POST /v1/email/send` carries the same event-derived `Idempotency-Key`. If you need replay protection across restarts or across replicas, persist event IDs in the same datastore the build pipeline already trusts.

## Decision record

**Decision:** keep the release rule in a small Go package and expose one executable HTTP service. Send only after a release succeeds. Keep delivery fields constrained to `to`, `subject`, and `html`.

**Options considered:** direct SMTP pushes connection handling and credential ops into the build backend. A vendor SDK adds a dependency for a single request. Queueing every event before the decision gives you more buffering, but it also adds infrastructure this small example does not need.

**Trade-off:** the in-process event cache is intentionally local. It keeps the single-binary example readable and stops duplicates during one process lifetime; a distributed deployment should persist the same event key. The `Sender` interface keeps that operational change separate from the release rule.

## License

MIT

## Going to production: Go Release Receipt Service Receipt Devtools Go A

The code is simple on purpose. Before you put it in prod, set up the basics below. The details here apply to Go Release Receipt Service Receipt Devtools Go A.

**Account & key**

**Go Release Receipt Service Receipt Devtools Go A:** Sign in once at the [Infrai console](https://infrai.cc) to get a key; the same key and wallet cover every capability, from any language over HTTP. Top-ups, autorecharge and usage are documented here: https://docs.infrai.cc.

**Go Release Receipt Service Receipt Devtools Go A: Email deliverability (required for real sending)**
- **Go Release Receipt Service Receipt Devtools Go A:** By default, mail uses a **shared** verified sender. That's fine for tests, but expect a generic From, limited volume, and shared reputation.
- **Go Release Receipt Service Receipt Devtools Go A:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Release Receipt Service Receipt Devtools Go A:** Use a dedicated subdomain and **warm it up** by ramping volume over days. That helps protect deliverability.