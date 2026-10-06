---
name: observability
description: Add production-grade logging, metrics, tracing, health checks, correlation IDs, structured errors, and operational diagnostics to backend systems. Use when making services production-ready, debugging distributed systems, or reviewing operational visibility.
---

# Backend Observability

Design observability around questions operators need answered.

## Logs
Use structured logs with:
- timestamp
- severity
- service
- operation
- request/correlation ID
- relevant entity ID
- error details without secrets

Avoid logging passwords, tokens, authorization headers, or sensitive payloads.

## Metrics
Measure meaningful signals:
- request rate
- error rate
- latency
- queue depth
- worker utilization
- database latency
- external dependency failures
- Kafka consumer lag where applicable

## Tracing
Propagate correlation/trace context across service and asynchronous boundaries where supported.

## Health
Separate:
- liveness: process is alive
- readiness: process can serve traffic
- dependency health where appropriate

Do not make liveness fail merely because PostgreSQL is temporarily unavailable.

## Alerts
Prefer symptom-based alerts over noisy implementation metrics.
Every important alert should have an operator action or investigation path.
