# Skills Analysis — `main_skills/.agents/skills`

> 38 skills total, organized into **4 functional groups**: Engineering Flow, Codebase Health, Writing, and Utilities/Setup.

---

## 🗺️ The Big Picture — How Skills Connect

The `ask-matt` skill is the **master router** — it maps every skill onto a single mental model of how work travels from idea to shipped code.

```
On-Ramps                Main Flow                         Standalone
─────────               ──────────────────────────────    ──────────────
triage ──────┐          grill-with-docs                   grill-me
             ├───────►  ├── (prototype detour via handoff) prototype
diagnosing   │          to-spec                           research
-bugs ───────┤          to-tickets                        to-questionnaire
             │          implement / implement-spec         wizard
wayfinder ───┘              └── tdd + code-review         wait-what
                         retro (closes every loop)        teach
                                                          loop-me
Vocabulary underneath:
  domain-modeling  ·  codebase-design
```

---

## 🔵 Group 1 — The Main Engineering Flow (Idea → Ship)

### 1. `ask-matt`
**The router / meta-skill.** Does not invoke the model — it's a reference map.

- Describes every skill and how they connect into flows
- Defines the concept of **"main flow"** (idea → grill → spec → tickets → implement → retro) and two **on-ramps** (triage, diagnosing-bugs, wayfinder)
- Defines **phase boundaries**: when to Continue / `/clear` / `/handoff` / spawn subagent / `/compact`
- Must-read before your first session to understand the entire system

---

### 2. `grilling`
**The core interview primitive.** Used by many other skills internally.

**Workflow:**
1. Maps the topic as a **design tree** — every decision branches into downstream decisions
2. Each round asks only the **frontier** (questions whose prerequisites are already settled)
3. Each question comes with the agent's recommended answer
4. Waits for user answers → recomputes frontier → next round
5. Session is done when frontier is empty (no silently-assumed decisions remain)

> Facts are the agent's job (it dispatches sub-agents for those). Decisions belong to the user.

---

### 3. `grill-me`
**Stateless version of grilling.** Thin wrapper — just calls `grilling`.

- Use when you have **no working directory** (planning a design, a blog post, a document)
- Saves nothing, creates no docs
- If you *are* in a repo → use `grill-with-docs` instead

---

### 4. `grill-with-docs`
**Stateful version of grilling.** Calls both `grilling` AND `domain-modeling`.

**Workflow:**
- Runs the same relentless interview as `grill-me`
- Also maintains `GLOSSARY.md` and writes ADRs as decisions crystallize
- Leaves a paper trail in the repo
- Entry point for the main engineering flow when working inside a repository

---

### 5. `domain-modeling`
**Active vocabulary discipline.** Called by `grill-with-docs`, `triage`, `improve-codebase-architecture`, `wayfinder`.

**Workflow during a session:**
1. Challenges any term that conflicts with `GLOSSARY.md`
2. Sharpens fuzzy/overloaded terms (e.g. "account" doing three jobs)
3. Stress-tests domain relationships with concrete edge-case scenarios
4. Cross-references terminology against the actual code
5. Updates `GLOSSARY.md` *inline* as terms are resolved (no batching)
6. Offers ADRs only when: decision is hard-to-reverse AND surprising without context AND a real trade-off

> `GLOSSARY.md` must stay devoid of implementation details — it's a glossary, not a spec.

---

### 6. `to-spec`
**Synthesises the conversation into a publishable spec.** No interview — pure synthesis.

**Workflow:**
1. Explores the repo to understand the current codebase state
2. Sketches test seams (prefers existing seams, proposes at the highest level possible)
3. Confirms seam choices with the user
4. Writes the spec using the template: Problem Statement → Solution → User Stories → Implementation Decisions → Testing Decisions → Out of Scope → Further Notes
5. Publishes to the issue tracker with `ready-for-agent` label

---

### 7. `to-tickets`
**Breaks a spec into a task graph of tracer-bullet tickets.**

**Workflow:**
1. Gathers context from the conversation (fetches referenced spec if provided)
2. Optionally explores the codebase
3. Drafts **vertical slice** tickets — each cuts through ALL layers (schema, API, UI, tests) and is demoable on its own
4. Assigns **blocking edges** between tickets, creating a **task graph**
5. Quizzes the user on granularity and blocking relationships
6. Publishes tickets to the configured tracker:
   - **Local**: `.scratch/<feature>/issues/<NN>-<slug>.md`
   - **GitHub/GitLab**: native issues with blocking links + `ready-for-agent` label

> Wide refactors use **expand–contract** sequencing (expand → migrate batches → contract) rather than vertical slicing.

---

### 8. `implement`
**Implements one ticket or spec in a single session.**

**Workflow:**
1. Reads the ticket/spec
2. Drives **`/tdd`** at pre-agreed seams
3. Runs typechecking regularly, single test files regularly, full suite once at the end
4. Calls **`/code-review`** when done
5. Commits to the current branch

---

### 9. `implement-spec`
**Orchestrates the full build of an entire spec using parallel subagents.**

**Workflow:**
1. Reads spec + tickets to understand the task graph
2. Optionally runs an **exploration subagent** to pre-gather codebase context
3. Creates the **integration branch**, opens a draft PR after the first merge
4. Spawns parallel **implementer subagents** — each:
   - Confirms worktree is based on integration branch
   - Runs `/tdd` to build its ticket
   - Merges integration branch tip before reporting done
5. A **merger subagent** merges each completed ticket to the integration branch
6. As tickets complete, advances the **frontier** and starts more implementers
7. Final **`/code-review`** pass on the integration branch
8. Marks PR ready for review, cleans up all worktrees

---

### 10. `tdd`
**Test-driven development discipline.**

**Core principle:** Tests verify behaviour through public interfaces, never implementation details.

**Workflow:**
1. Reads `GLOSSARY.md` to match test names to domain language
2. Agrees **seams** (public boundaries to test at) with the user BEFORE writing any test
3. Red → Green loop per slice:
   - Write failing test at the seam
   - Write minimum code to pass
   - Repeat (refactoring is a review concern, not part of this loop)
4. Avoids anti-patterns: implementation-coupled tests, tautological assertions, horizontal slicing

---

### 11. `code-review`
**Two-axis review: Standards + Spec, run in parallel subagents.**

**Workflow:**
1. Pins a fixed point (`git diff <fixed-point>...HEAD`)
2. Identifies the spec source (issue references, spec files, user-provided path)
3. Identifies standards sources (`CODING_STANDARDS.md`, `CONTRIBUTING.md`)
4. Spawns two parallel subagents simultaneously:
   - **Standards subagent**: checks against repo standards + a fixed Fowler code-smell baseline (Mysterious Name, Duplicated Code, Feature Envy, Data Clumps, etc.)
   - **Spec subagent**: checks for missing requirements, scope creep, wrong implementations
5. Aggregates findings under `## Standards` and `## Spec` headings (deliberately kept separate)

> The two axes are kept separate so one can't mask the other (code that follows standards but implements the wrong thing is a Spec fail, not a pass).

---

### 12. `pr`
**Writes the PR body.** Model-invoked (the agent calls it automatically when writing a PR).

**Template structure:**
- **Summary**: smallest visual that makes the key point (pseudocode, call tree, component tree, file tree, Mermaid diagram, diff-sketch)
- **Evidence**: before/after screenshots or test results
- **Merge Danger**: one-way vs two-way door + blast radius

---

### 13. `retro`
**Retrospective on a coding session — improves the agent's environment, not the code.**

**Workflow:**
1. Reads `writing-for-agents` for style guide
2. Reads the session's primary sources (logs, commits)
3. Looks for candidates in 7 categories: Navigation · Automated Checks · Coding Standards · Global AGENTS.md · Tool Economy · No-ops · Information Access
4. Presents candidates to user in order of severity

> Key insight: mechanical violations → deterministic checks (linters, pre-commit hooks). Judgement calls → `CODING_STANDARDS.md`. Review agents enforce standards; implementation agents should focus on building.

---

## 🟡 Group 2 — On-Ramps (merge onto main flow)

### 14. `triage`
**Moves incoming issues through a triage state machine.** For issues you *didn't* create.

**Roles:**
- Categories: `bug`, `enhancement`
- States: `needs-triage` → `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`

**Workflow per issue:**
1. Gather context (body, comments, labels, diffs for PRs)
2. Check for redundancy (already implemented?) and prior rejections (`.out-of-scope/`)
3. Recommend category + state with reasoning
4. Verify the claim (reproduce bugs, test PR diffs)
5. Grill if needed (calls `grilling` + `domain-modeling`)
6. Apply outcome: post agent brief, post needs-info, close as wontfix, etc.

> Every triage comment must start with `> *This was generated by AI during triage.*`

---

### 15. `diagnosing-bugs`
**Structured discipline for hard bugs.** 6 strict phases.

**Phases:**
1. **Build a feedback loop** ← *"This is the skill."* Must produce one command that goes red on the specific bug (deterministic, fast, agent-runnable). No moving to phase 2 without it.
2. **Reproduce + minimise** — confirm the right bug, shrink to smallest scenario that still goes red
3. **Hypothesise** — generate 3–5 ranked, *falsifiable* hypotheses before testing any. Show to user.
4. **Instrument** — one variable at a time, tag all debug logs `[DEBUG-xxxx]` for easy cleanup
5. **Fix + regression test** — write the test BEFORE the fix (if a correct seam exists)
6. **Cleanup** — remove debug logs, delete prototypes, state the winning hypothesis in the commit

---

### 16. `wayfinder`
**For huge, foggy efforts too big for one session.** Creates a decision map, resolves one ticket at a time.

**Key concepts:**
- **Map**: a single issue labelled `wayfinder:map`, listing all decisions
- **Tickets**: child issues of the map, each resolving one decision (not a deliverable)
- **Fog of war**: the dim view of decisions you can't yet pin down — written to "Not yet specified"
- **Frontier**: open, unblocked, unclaimed tickets — the edge of the known
- **Ticket types**: Research (AFK) · Prototype (HITL) · Grilling (HITL) · Task (HITL or AFK)

**Workflow — Chart the map:**
1. Name the destination (calls `grilling` + `domain-modeling`)
2. Map breadth-first across the whole space
3. Create the map issue and initial tickets
4. Wire blocking edges
5. Fire research subagents in parallel

**Workflow — Work through the map:**
1. Load the map (low-res view)
2. Claim the first frontier ticket
3. Resolve it (zoom as needed, call skills from Notes)
4. Record the resolution, close the issue, append to Decisions-so-far
5. Graduate fog into new tickets

> Never resolve more than one ticket per session (except research tickets). When the way is clear → hand off to `/to-spec`.

---

## 🟢 Group 3 — Codebase Health

### 17. `improve-codebase-architecture`
**Scans the codebase for deepening opportunities and generates an HTML report.**

**Workflow:**
1. **Scope** — focus on hot spots (files that keep changing in git history)
2. **Explore** via a sub-agent looking for: shallow modules, pure-function testability extractions, tight coupling, untested code
3. **Generate HTML report** to `$TMPDIR/architecture-review-<timestamp>.html` — each candidate gets a card with: Files, Problem, Solution, Benefits, Before/After diagram, Recommendation strength (`Strong` / `Worth exploring` / `Speculative`)
4. User picks a candidate
5. **Grilling loop** calls `grilling` + `domain-modeling` to walk the decision tree for the chosen candidate

Uses `codebase-design` vocabulary throughout.

---

### 18. `codebase-design`
**Shared vocabulary for designing deep modules.** Referenced by other skills, not typically run standalone.

**Key terms (use exactly, no substitutions):**
| Term | Meaning |
|------|---------|
| **Module** | Anything with an interface and implementation (function, class, package…) |
| **Interface** | Everything a caller must know (types + invariants + errors + ordering + perf) |
| **Depth** | Leverage: large behaviour behind a small interface |
| **Seam** | Where the interface lives; where you can swap behaviour without editing there |
| **Adapter** | A concrete thing that fills a seam's interface |
| **Leverage** | What callers get from depth — more capability per unit of interface learned |
| **Locality** | What maintainers get — change, bugs, knowledge concentrate in one place |

**Design tests:**
- *Deletion test*: would deleting this concentrate complexity? If yes, it's earning its keep.
- *"The interface is the test surface"*: if you want to test past the interface, the module is wrong-shaped.
- *"One adapter = hypothetical seam, two = real"*: don't introduce a seam unless something actually varies.

---

## 🟣 Group 4 — Standalone Skills

### 19. `handoff`
**Compacts a conversation into a markdown file for a fresh agent to pick up.**

- Saves to the OS temp directory (not the workspace)
- Includes a "suggested skills" section
- References existing artifacts by path (no duplication)
- Redacts sensitive information

---

### 20. `claude-handoff`
**Like `handoff`, but immediately launches a background agent.**

- Runs `claude --bg --name "<descriptive name>" "<handoff summary>"`
- The new agent starts in the current directory and is manageable via `claude agents`
- Tailors the summary if the user passed an argument describing the next session's focus

---

### 21. `prototype`
**Answers a design question with throwaway code.**

**Two branches:**
- **"Does this logic feel right?"** → `LOGIC.md` — single shareable HTML with a state machine explorer
- **"What should this look like?"** → `UI.md` — several UI variations switchable via URL param

**Rules:**
1. Clearly marked as throwaway, located near the module it prototypes
2. Trivial to run (one command)
3. No persistence by default (state in memory)
4. Skip polish — no tests, no error handling beyond "runnable"
5. Surface the state visibly
6. Capture when done: commit to a `prototype/<name>` branch, leave a context pointer on the issue

---

### 22. `research`
**Delegates reading legwork to a background agent.**

**Workflow:**
1. Spins up a background agent
2. Agent investigates against **primary sources** (official docs, source code, specs) — not secondary write-ups
3. Writes findings to a single Markdown file with citations
4. Saves where the repo already keeps such notes

You keep working while it reads.

---

### 23. `to-questionnaire`
**Writes a questionnaire for knowledge only someone else holds.**

**Workflow:**
1. Asks about the *recipient* (role, expertise, relationship) — one exchange
2. Asks what you *need back* — concrete decisions/facts — one exchange
3. Writes questionnaire to `to-questionnaire-<slug>.md`, questions ordered most-important-first, grouped by theme, each with an answer stub and a one-line "why this matters"

> Grills the *send*, not the subject. The questions target the gap between what the recipient knows and what you need.

---

### 24. `wait-what`
**Corrective for a message that didn't land.** One-liner: asks the agent to re-pitch its last message in ASD-STE100 Simplified Technical English using `GLOSSARY.md` vocabulary.

---

### 25. `chief-of-staff`
**Long-running strategic coordination skill** — pursues a goal across many sessions by coordinating subagents.

- Owns the **strategic view**, subagents own the tactical
- Communicates via **context pointers** (research notes, commits) not duplicated content
- Suggests recurring schedules where useful
- Acts as the "gardener" of the environment agents operate in

---

### 26. `loop-me`
**Designs recurring personal/professional workflows.** Stateful, grilling-based.

**Vocabulary:**
- **Loop**: a recurring pattern in the user's life worth delegating
- **Workflow**: the spec of one loop, stored in `workflows/*.md`
- **Trigger**: event-based or schedule-based
- **Checkpoint**: human-in-the-loop decision point
- **Push right**: defer the checkpoint as far as possible, do maximal work first
- **Brief**: tight, decision-ready summary presented at a checkpoint

A workflow spec is done when an implementer agent could build it without asking a single question.

---

### 27. `wizard`
**Generates interactive bash scripts for steps only a human can take.**

Use for: provisioning infrastructure, credentials setup, CI secrets, third-party dashboards, one-off migrations.

**Workflow:**
1. Scope the procedure (reads `.env`, `.env.example`, `docker-compose*`, CI workflows)
2. Map each stage's precise journey (which URL, what to click, where the value appears)
3. Author from `template.sh` — uses library helpers: `stage`, `open_url`, `ask_secret`, `write_env`, `set_secret`, `confirm`
4. Verify with `bash -n` + `shellcheck`, trace statically

> Never run end-to-end yourself — the script opens browsers and blocks on human input.

---

### 28. `teach`
**Multi-session, stateful learning workspace.**

**Workspace files:**
- `MISSION.md` — why the user wants to learn (grounds all teaching)
- `./reference/*.html` — beautiful cheat sheets / reference docs
- `RESOURCES.md` — high-quality primary sources
- `./learning-records/*.md` — key insights (like ADRs for learning)
- `./lessons/*.html` — self-contained lessons (the primary unit)
- `./assets/*` — reusable components across lessons

**Teaching philosophy:**
- Knowledge → Skills → Wisdom (community)
- Prioritize **storage strength** (long-term retention) over fluency strength
- Use: retrieval practice · spaced repetition · interleaving
- Zone of Proximal Development: challenge the user "just enough"

---

## 🔧 Group 5 — Setup & Tooling Skills

### 29. `setup-matt-pocock-skills`
**One-time repo configuration that all other engineering skills depend on.**

Configures:
- **Issue tracker**: GitHub / GitLab / Local markdown / Other
- **Triage label vocabulary**: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`
- **Domain docs layout**: `GLOSSARY.md` + `docs/adr/` (single-context) or `GLOSSARY-MAP.md` (multi-context)

Writes to `CLAUDE.md` or `AGENTS.md` and creates `docs/agents/issue-tracker.md`, `docs/agents/domain.md`, `docs/agents/triage-labels.md`.

> Run this *before* your first use of triage, to-spec, to-tickets, implement-spec, or wayfinder.

---

### 30. `setup-pre-commit`
**Sets up Husky + lint-staged + Prettier pre-commit hooks.**

**Steps:** Detect package manager → Install dependencies → `npx husky init` → Write `.husky/pre-commit` → Create `.lintstagedrc` → Create `.prettierrc` if missing → Verify → Commit

Hook runs: `lint-staged` (staged-only, fast) → `typecheck` → `test`

---

### 31. `setup-ts-deep-modules`
**Enforces deep module architecture in TypeScript repos using dependency-cruiser.**

Enforces 4 rules at `error` level:
1. **Entry-point boundary**: outsiders can only import package root files (never subfolders)
2. **Intra-package freedom**: a package's own files import each other freely
3. **Tests through entry points**: tests may never deep-import package internals
4. **No cycles**

**Steps:** Detect env → Install dependency-cruiser → Write `.dependency-cruiser.cjs` → Wire `lint:boundaries` script → Scaffold example package → **Prove the rules bite** (observe pass → fail on deep import → pass again) → Document in README + add context pointer to `AGENTS.md`

---

### 32. `git-guardrails-claude-code`
**Installs a Claude Code PreToolUse hook to block dangerous git commands.**

Blocked: `git push`, `git reset --hard`, `git clean -f/fd`, `git branch -D`, `git checkout .`, `git restore .`

**Steps:** Ask scope (project vs global) → Copy `block-dangerous-git.sh` → `chmod +x` → Add to `settings.json` → Ask about customization → Verify with a test echo

---

### 33. `migrate-to-shoehorn`
**Migrates test `as` assertions to `@total-typescript/shoehorn`.**

| Function | Use case |
|----------|---------|
| `fromPartial()` | Pass partial data that still type-checks (replaces `as Type`) |
| `fromAny()` | Pass intentionally wrong data (replaces `as unknown as Type`) |
| `fromExact()` | Force full object |

**Workflow:** Gather requirements → Install → grep for `as` assertions in test files → Replace → Add imports → Typecheck

---

### 34. `scaffold-exercises`
**Creates exercise directory structures for course content.**

Naming: `XX-section-name/` → `XX.YY-exercise-name/` → `problem/` + `solution/` + `explainer/`

Each subfolder needs a non-empty `readme.md`. Runs `pnpm ai-hero-cli internal lint` after creation and iterates until it passes. Uses `git mv` for renames to preserve history.

---

## ✍️ Group 6 — Writing Skills

### 35. `writing-fragments` — *Explore*
**Mines raw fragments from conversation — no structure yet.**

- Runs a grilling session and captures any piece of text that might survive to the final article
- Appends fragments to a markdown file separated by `---`
- Never imposes structure, outlines, or phases
- The most valuable fragment type: a **leading word** (a compact metaphor the whole piece can hang on)

---

### 36. `writing-shape` — *Exploit*
**Shapes a pile of raw fragments into a structured article, paragraph by paragraph.**

**Workflow:**
1. Reads the pile in full
2. Establishes prerequisites (what reader already knows — "grounded" concepts)
3. Drafts 2–3 candidate openings, user picks one
4. Grows paragraph by paragraph — each block may only lean on grounded concepts
5. Appends to article file immediately after each agreed block
6. Uses the pile as a quarry: paraphrase, split, recombine, quote

---

### 37. `writing-beats` — *Exploit with choose-your-own-adventure*
**Like `writing-shape` but beat-by-beat with branching paths.**

**Workflow:**
1. Establish prerequisites (grounded concepts)
2. Write 2–3 candidate starting beats (different entry points)
3. User picks one → write only that beat → preview what it unlocks
4. Offer 2–3 candidate next beats
5. Loop until done

**Grounding rule**: Every concept must be grounded (by prerequisite or by an earlier beat) before a beat can lean on it. This is what drives the choose-your-own-adventure: picking a beat that grounds concept X unlocks all beats waiting on X.

---

### 38. `writing-for-agents`
**Reference for writing documents that agents consume** (skills, AGENTS.md, docs).

**Key concepts:**
- **Context pointer**: a reference in context that names out-of-context material + when to reach it
- **Progressive disclosure**: push material behind a pointer so the top stays legible
- **Leading word**: a compact concept from model pretraining that anchors a region of behaviour in few tokens (e.g. `tight`, `red`, `tracer bullets`)
- **Completion criterion**: the condition that tells the agent "done". Must be clear and demanding.
- **Pruning**: single source of truth · no caches of the environment · no no-ops · no sediment

---

## 🔄 Complete Workflow Summary

```
[Precondition: run setup-matt-pocock-skills once]

GREENFIELD / LARGE FEATURE (foggy)          NORMAL FEATURE IDEA
──────────────────────────────────          ──────────────────
wayfinder                                   grill-with-docs
  │ (map clear)                               │
  ▼                                           │
to-spec ◄───────────────────────────────────►│
  │                                           │
to-tickets                                    │
  │                                           │
  ├── implement (per ticket, fresh context)   implement (same window)
  │      └── tdd + code-review                  └── tdd + code-review
  │
  └── implement-spec (parallel subagents)
         └── tdd (per subagent) + code-review (integration branch)

pr (written automatically when pushing)

retro (at end of session, before clearing)

──────────────────────────────────────────────────────────────────
INCOMING BUGS/REQUESTS                      HARD BUGS
──────────────────────────────────          ──────────────
triage → implement                          diagnosing-bugs → retro
                                              └── improve-codebase-architecture
                                                    (if no seam exists)
```
