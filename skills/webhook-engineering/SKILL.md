---
name: webhook-engineering
description: Design and implement reliable webhook producers and consumers. Use whenever a project receives, sends, retries, verifies, persists, queues, or observes webhook events.
---

# Webhook Engineering

Treat webhook delivery as an unreliable distributed interaction.

## Receiver flow

```text
HTTP request
  ↓
Read raw body
  ↓
Verify signature
  ↓
Validate envelope
  ↓
Persist event / deduplicate
  ↓
Return fast acknowledgment
  ↓
Asynchronous processing
```

Do not perform long business workflows before acknowledging a provider unless the provider explicitly requires synchronous processing.

## Idempotency
Use a provider event ID or deterministic idempotency key.
Persist processing state.
Assume duplicate delivery.

## Security
- Verify signatures against the exact raw payload.
- Validate timestamps/replay windows where supported.
- Protect against oversized bodies.
- Do not trust event fields merely because they came from a webhook.

## Delivery
For outbound webhooks define:
- retry schedule
- maximum attempts
- timeout
- response success range
- signing algorithm
- event ID
- delivery ID
- delivery state
- dead-letter behavior

## Reliability
Model states explicitly, for example:
PENDING → DELIVERING → DELIVERED
PENDING → RETRYING → DELIVERING
RETRYING → FAILED

Persist enough information to debug and replay deliveries safely.
