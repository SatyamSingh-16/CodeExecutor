---
name: langchain-backend
description: Design and implement production backend applications using LangChain, LLMs, tools, retrieval, agents, structured output, memory, and provider integrations. Use whenever a backend project includes LangChain or an LLM workflow.
---

# LangChain Backend Engineering

Treat the LLM layer as one subsystem of the backend, not as the entire architecture.

## Architecture
Prefer explicit boundaries:

```text
HTTP/API
   ↓
Application Service
   ↓
LLM Orchestration
   ├── model/provider
   ├── prompt
   ├── tools
   ├── retrieval
   └── structured output
   ↓
Domain / Infrastructure
```

Keep provider-specific code isolated where practical.

## Reliability
- Validate model output.
- Prefer structured output/schema validation for machine-consumed results.
- Set timeouts.
- Bound retries.
- Handle provider rate limits.
- Design fallback behavior explicitly.
- Never assume an LLM call is deterministic.
- Log metadata, not secrets or sensitive prompts/responses.

## Prompt/tool design
- Keep system instructions explicit.
- Define tool inputs and outputs narrowly.
- Validate tool arguments.
- Do not give an agent unrestricted access to dangerous operations.
- Require confirmation for consequential side effects where appropriate.

## RAG
Separate:
ingestion → chunking → embeddings → indexing → retrieval → reranking/context assembly → generation.

Measure retrieval quality separately from generation quality.

## Evaluation
Create representative test cases for:
- correctness
- refusal/safety boundaries
- malformed tool calls
- hallucination
- retrieval failures
- provider errors
- latency/cost

Do not hide business logic inside giant prompts when deterministic code can enforce it.
