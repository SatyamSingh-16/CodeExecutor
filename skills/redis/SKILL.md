---
name: redis
description: Design and implement Redis usage for backend systems including caching, rate limiting, distributed coordination, idempotency, temporary state, queues, TTLs, and invalidation. Use whenever Redis is proposed or existing Redis code needs review.
---

# Redis Engineering

Use Redis for explicitly defined ephemeral or coordination workloads.

## Choose the pattern
- Cache: reduce repeated expensive reads.
- Rate limiting: control request frequency.
- Idempotency: remember processed request keys for a bounded period.
- Temporary state: short-lived workflow/session state.
- Distributed coordination: only with clearly defined correctness requirements.

Do not use Redis as the default replacement for PostgreSQL.

## Rules
- Every temporary key needs a TTL unless it is intentionally persistent.
- Define key naming conventions.
- Define serialization formats.
- Decide cache invalidation strategy before implementing caching.
- Handle Redis outages according to the application's availability requirements.
- Avoid cache stampedes with appropriate TTL jitter, locking, or request coalescing where needed.
- Never treat cache state as authoritative unless explicitly designed that way.

## Review
For every Redis feature document:
- key format
- value format
- TTL
- consistency model
- failure behavior
- eviction implications
- memory growth
