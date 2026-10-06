---
name: testing
description: Design and implement reliable tests for backend systems. Use whenever adding features, debugging failures, reviewing code, creating unit/integration tests, testing concurrency, persistence, APIs, workers, or external integrations.
---

# Backend Testing

Test behavior, contracts, invariants, and failure modes.

## Test layers
Use the smallest effective layer:
- unit tests for pure/domain/service behavior
- handler/API tests for HTTP contracts
- repository integration tests for real database behavior
- integration tests for component interaction
- end-to-end tests only for critical flows

## Go conventions
Prefer table-driven tests where cases share structure.
Use subtests for readable case isolation.
Use `t.Helper()` in test helpers.
Use deterministic fixtures.

## Concurrency
For concurrent systems test:
- parallel work
- cancellation
- shutdown
- queue saturation
- duplicate submission
- race conditions
- goroutine leaks where practical

Use the race detector for concurrency-heavy code.

## Failure-first testing
For each feature ask:
- malformed input?
- missing dependency?
- timeout?
- duplicate request?
- partial failure?
- database constraint violation?
- context cancellation?
- restart during operation?

Do not only test happy paths.

## Test quality
Avoid tests that merely mirror implementation details. Assert externally meaningful behavior.
