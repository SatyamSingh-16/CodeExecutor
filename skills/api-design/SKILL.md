---
name: api-design
description: Design, implement, review, and document HTTP/REST APIs. Use whenever adding endpoints, request/response models, validation, status codes, pagination, filtering, errors, authentication boundaries, idempotency, or API documentation.
---

# API Design

Design APIs as stable contracts.

## Endpoint checklist
For each endpoint define:
- method and path
- purpose
- authentication/authorization
- path parameters
- query parameters
- request body
- validation rules
- success status and response schema
- error statuses and error schema
- idempotency requirements
- pagination/filter/sort behavior where relevant

## HTTP semantics
Use status codes intentionally:
- 200 successful read/update
- 201 resource created
- 202 accepted for asynchronous processing
- 204 successful operation with no body
- 400 malformed/invalid request
- 401 unauthenticated
- 403 authenticated but forbidden
- 404 resource not found
- 409 state/conflict violation
- 422 semantically invalid input when appropriate
- 429 rate limited
- 500 unexpected server failure

Do not expose internal stack traces or database errors.

## Design rules
- Use nouns for resources.
- Keep response schemas predictable.
- Validate at the boundary.
- Do not leak persistence models directly when API contracts differ.
- Design asynchronous APIs around job IDs/status resources when work is not immediate.
- Document examples for important endpoints.

## Review
Check consistency across endpoints before adding new patterns.
