---
name: docker
description: Containerize backend applications and design local/production container workflows. Use when adding Dockerfiles, Compose files, service dependencies, health checks, networking, multi-stage builds, or container deployment configuration.
---

# Docker Engineering

Build small, reproducible, secure images.

## Dockerfile principles
- Use multi-stage builds for compiled Go applications.
- Build with a pinned/controlled toolchain.
- Keep runtime images minimal.
- Run as a non-root user when practical.
- Copy only required artifacts.
- Do not bake secrets into images.

## Compose
Use Compose for local multi-service development when useful:
- application
- PostgreSQL
- Redis
- Kafka
- supporting services

Define health checks for dependencies where startup ordering matters.

## Configuration
Pass environment-specific configuration at runtime.
Never commit production credentials.

## Networking
Use service names for Compose-to-Compose communication.
Do not hardcode localhost for dependencies running in separate containers.

## Validation
Check:
- build succeeds
- application starts
- health endpoint works
- dependencies are reachable
- graceful shutdown works
