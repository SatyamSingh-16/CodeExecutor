# Personal Backend Engineering Skills

Reusable Agent Skills for backend projects.

## Skills

- project-architect — requirements → architecture
- project-scaffolder — architecture → repository structure
- golang-backend — idiomatic Go backend engineering
- api-design — HTTP/REST contracts
- postgres — PostgreSQL persistence
- redis — Redis caching/coordination
- kafka — event-driven systems
- docker — containerization
- testing — backend testing strategy
- security — backend security
- observability — production visibility
- langchain-backend — LangChain/LLM backend architecture
- webhook-engineering — reliable webhook systems
- code-review — senior backend review

## Recommended workflow

1. Use `project-architect` before writing substantial code.
2. Review/approve the architecture.
3. Use `project-scaffolder`.
4. Activate only the technical Skills required by the project.
5. Implement in small vertical slices.
6. Use `testing` continuously.
7. Use `code-review` before major commits.
8. Use `security` and `observability` before calling a project production-ready.

## Suggested project composition

Code Executor:
project-architect + project-scaffolder + golang-backend + postgres + testing + docker + observability + code-review

Webhook Platform:
project-architect + project-scaffolder + golang-backend + postgres + redis + webhook-engineering + security + testing + docker + observability + code-review

LLM/LangChain Backend:
project-architect + project-scaffolder + golang-backend + langchain-backend + postgres + redis + security + testing + observability + code-review
