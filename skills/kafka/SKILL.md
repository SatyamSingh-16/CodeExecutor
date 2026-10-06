---
name: kafka
description: Design and implement Kafka-based event-driven backend systems. Use when introducing Kafka topics, producers, consumers, partitions, consumer groups, retries, ordering, delivery semantics, event schemas, or failure handling.
---

# Kafka Engineering

Treat Kafka as a distributed log and event transport, not a generic database.

## Design before coding
Define:
- event type
- topic
- partition key
- partition count rationale
- producer semantics
- consumer group
- ordering requirement
- retry strategy
- dead-letter strategy
- retention
- schema/versioning
- observability

## Partitioning
Partitioning determines parallelism and ordering boundaries.
If ordering is required for an entity, use a stable key that maps that entity's events to the same partition.

## Consumer behavior
Consumers must tolerate:
- duplicate delivery
- retries
- consumer restarts
- rebalancing
- poison messages
- downstream outages

Design handlers to be idempotent when possible.

## Delivery semantics
Do not casually claim exactly-once semantics. Explain what is guaranteed by Kafka and what the application must guarantee.

## Failure handling
Define retry delays, maximum attempts, DLQ behavior, offset commit timing, and recovery procedures.

## Go
Make consumer lifecycle explicit: start, cancellation, rebalance handling, shutdown, and error propagation.
