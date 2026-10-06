---
name: project-scaffolder
description: Create or modify a clean backend project structure from an approved architecture. Use when initializing a new Go backend, creating directories, adding packages, wiring configuration, migrations, tests, Docker files, or other project boilerplate.
---

# Project Scaffolder

Turn an approved architecture into a minimal working repository structure.

## Rules
- Scaffold only components justified by the architecture.
- Never create empty speculative packages just because they are common.
- Keep executable entrypoints under `cmd/`.
- Keep application internals under `internal/` when appropriate.
- Keep database migrations separate from application code.
- Keep deployment files at repository root.
- Keep documentation under `docs/`.
- Keep tests close to code unless integration/system tests need a dedicated location.
- Establish dependency injection at composition/root wiring.

## Preferred Go baseline

```text
project/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── domain/
│   ├── handler/
│   ├── service/
│   ├── repository/
│   ├── middleware/
│   ├── config/
│   └── infrastructure/
├── migrations/
├── docs/
├── tests/
├── Dockerfile
├── compose.yaml
├── go.mod
└── README.md
```

Adapt this structure instead of copying it blindly.

## Workflow
1. Read the architecture.
2. List required components.
3. Generate directories and files.
4. Add minimal compileable wiring.
5. Add configuration boundaries.
6. Add migration structure if persistence exists.
7. Add test scaffolding.
8. Run formatting/build/tests if tools are available.
9. Report exactly what was created and why.
