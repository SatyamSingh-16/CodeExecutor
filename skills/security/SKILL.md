---
name: security
description: Review and implement backend security controls including authentication, authorization, input validation, secrets, TLS, CORS, CSRF considerations, rate limiting, injection prevention, webhook verification, and secure configuration. Use for security reviews or whenever sensitive backend functionality is added.
---

# Backend Security

Apply defense in depth.

## Input and output
- Validate untrusted input at boundaries.
- Use parameterized SQL.
- Constrain request sizes.
- Avoid command injection and unsafe shell execution.
- Do not return secrets or internal stack traces.

## Authentication vs authorization
Authentication answers who the caller is.
Authorization answers what the caller may do.
Enforce authorization server-side for every protected operation.

## Secrets
- Never hardcode secrets.
- Never commit credentials.
- Use environment/secret management mechanisms.
- Rotate credentials when exposure is suspected.

## HTTP
- Use TLS in production.
- Configure CORS narrowly.
- Apply security headers where appropriate.
- Rate-limit sensitive endpoints.
- Set sensible timeouts.

## Webhooks
Verify signatures over the exact raw request body.
Use constant-time comparison.
Check timestamp/replay protections when the provider supports them.
Make processing idempotent.

## Review
For each new endpoint identify:
identity, permissions, input trust boundary, data exposure, abuse cases, logging/privacy implications, and failure behavior.
