---
name: golang-backend
description: Build and review production-grade Go backend services using idiomatic Go, net/http, concurrency primitives, context, interfaces, dependency injection, errors, packages, and testing. Use for Go APIs, workers, services, repositories, middleware, concurrent systems, and backend refactoring.
---

# Go Backend Engineering

Write idiomatic, maintainable Go.

## Principles
- Prefer standard library solutions when sufficient.
- Keep functions small and responsibilities explicit.
- Avoid unnecessary interfaces; define interfaces where consumers need abstraction.
- Propagate `context.Context` through request-scoped operations.
- Wrap errors with useful context and preserve error identity.
- Avoid global mutable state.
- Make goroutine ownership and shutdown explicit.
- Protect shared mutable state with appropriate synchronization.
- Prefer channels for communication/coordination and mutexes for protecting state when appropriate.
- Do not launch goroutines without a clear lifecycle.

## HTTP
- Keep handlers thin.
- Parse/validate transport input in handlers.
- Put business rules in services/domain logic.
- Put persistence in repositories.
- Return consistent HTTP errors.
- Set headers before writing response bodies.
- Respect request cancellation.

## Concurrency
For every goroutine ask:
1. Who starts it?
2. Who stops it?
3. What happens on cancellation?
4. Can it leak?
5. What happens during shutdown?
6. Is shared state synchronized?

Use `sync.WaitGroup`, channels, mutexes, contexts, and worker pools deliberately.

## Quality
Run `gofmt`, `go vet`, tests, and race detection where appropriate. Prefer table-driven tests for behavior-heavy code.
