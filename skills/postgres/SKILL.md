---
name: postgres
description: Design, implement, review, and troubleshoot PostgreSQL persistence for backend applications. Use whenever a project needs schemas, migrations, transactions, indexes, constraints, queries, connection pools, repository implementations, or PostgreSQL performance work.
---

# PostgreSQL Engineering

Treat PostgreSQL as a durable consistency boundary.

## Schema design
- Define primary keys deliberately.
- Use foreign keys for relational integrity.
- Add `NOT NULL` when absence is invalid.
- Use `CHECK` constraints for domain invariants that belong in the database.
- Choose indexes from query patterns, not speculation.
- Prefer explicit timestamps and consistent timezone handling.
- Store UUIDs when distributed identity is useful; do not use them automatically.

## Migrations
- Every schema change must be versioned.
- Keep migrations small and reversible where practical.
- Never modify an already-applied migration casually.
- Consider locking and table size for production migrations.

## Repository layer
- Keep SQL/persistence concerns inside repositories.
- Pass context to queries.
- Use transactions for multi-step state changes requiring atomicity.
- Handle `sql.ErrNoRows` or driver-specific equivalents explicitly.
- Avoid N+1 query patterns.

## Performance
Inspect query plans when performance matters.
Index columns used in selective WHERE, JOIN, ORDER BY, and uniqueness patterns.
Do not add indexes without considering write overhead.

## Safety
Never concatenate untrusted values into SQL. Use parameterized queries.
