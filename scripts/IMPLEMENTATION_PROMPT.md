# Implementation Task — Orchestrator

You are the **orchestrator** for implementing a mock Mailgun API service. You select the next task, then coordinate sub-agents to implement it using a test-driven development workflow.

## Your Role (Main Agent)

You are responsible for:
1. Reading the implementation plan and selecting the next task
2. Preparing context for sub-agents (relevant specs, existing code patterns)
3. Launching sub-agents in sequence
4. Updating the implementation plan after successful completion
5. Committing all changes

**You do NOT write implementation code or tests yourself.** You delegate that work to sub-agents via the Agent tool.

## Workflow

### Step 1: Select the Next Task

1. **Read the implementation plan** at `implementation_plan/implementation.md` to understand overall progress.
2. **Find the next unchecked task** (`- [ ]`) in the earliest incomplete phase. Work through phases in order — don't skip ahead unless a phase is fully complete.
3. **Read the relevant plan doc** linked from the phase header (e.g., `implementation_plan/domains.md`) for endpoint specs, field schemas, and behavioral details.
4. **Read the scratchpad** at `implementation_plan/scratchpad.md` for cross-cutting concerns, field casing rules, and API version discrepancies that affect the task.
5. **Read CLAUDE.md** for project conventions, tech stack, and commands.
6. **Read existing code** in `internal/` and `web/` to understand established patterns before delegating work.

Identify a **complete logical unit of work** — a cohesive subsection like "Domain CRUD", "Bounces", or "Event Querying". Don't pick a single checkbox in isolation if related checkboxes form a cohesive unit.

### Step 2: Launch Test Writer Sub-Agent

Spawn a sub-agent using the **Agent tool** (`subagent_type: "general-purpose"`) to write tests FIRST.

In your prompt to the test writer, include:
- The specific task description and which endpoints/features to test
- Relevant endpoint specs from the plan doc (HTTP methods, paths, request/response schemas, field names and casing)
- Cross-cutting concerns from the scratchpad (pagination style, field casing rules, auth patterns)
- Existing code patterns (paste relevant snippets from existing handlers, models, and tests so it follows the same style)
- File paths where tests should be created

The test writer sub-agent must:
- Write comprehensive Go tests using `testing` + `httptest`
- Cover happy paths, validation errors, edge cases, and pagination where applicable
- Tests should compile but are **expected to fail** (no implementation exists yet)
- Run `go vet ./...` to verify no syntax issues
- NOT write any implementation code — only test files

### Step 3: Launch Implementer Sub-Agent

After the test writer completes, spawn another sub-agent to implement the feature.

In your prompt to the implementer, include:
- The same task description and endpoint specs
- Which test files were written and where (so it knows what to make pass)
- Existing code patterns for models, handlers, and route registration
- The project's tech stack details (chi router, GORM with SQLite/Postgres, etc.)

The implementer sub-agent must:
- Create or modify GORM models as needed (in `internal/`)
- Implement handlers following the chi router patterns established in the codebase
- Register new routes in the server
- Run `go test ./...` and ensure **all tests pass**
- Run `go vet ./...` to check for issues
- NOT modify test files — only implementation code

### Step 4: Launch Reviewer Sub-Agent

After the implementer completes, spawn a sub-agent to review all changes from Steps 2 and 3.

In your prompt to the reviewer, include:
- A list of all new and modified files
- The task requirements (endpoint specs, expected behavior)
- Instructions to classify each finding by severity

The reviewer sub-agent must:
- Read ALL new and modified files
- Check for:
  - **Correctness**: Does the implementation match the Mailgun API spec? Are request/response formats right?
  - **Security**: SQL injection, input validation, auth bypass risks
  - **Test coverage**: Are important paths tested? Missing edge cases?
  - **Code quality**: Error handling, resource cleanup, naming consistency
  - **API shape**: Field names, casing, nesting match what the plan docs specify
- Classify each issue as: **CRITICAL**, **HIGH**, **MEDIUM**, or **LOW**
- Return a structured report with findings, or explicitly state "No high or critical issues found"

### Step 5: Fix Critical/High Issues (Conditional)

If the reviewer reports any **CRITICAL** or **HIGH** severity issues, spawn a fixer sub-agent.

In your prompt to the fixer, include:
- The specific CRITICAL/HIGH issues verbatim from the review
- The file paths that need changes
- Instructions to run `go test ./...` after fixes to ensure nothing breaks

The fixer sub-agent must:
- Address each CRITICAL and HIGH issue
- Run tests to verify fixes don't break anything
- Run `go vet ./...`

If there are only MEDIUM/LOW issues, skip this step — those can be addressed in future iterations.

### Step 6: Finalize

After all sub-agents complete successfully:

1. **Update the implementation plan** at `implementation_plan/implementation.md`:
   - Check off finished items (`- [ ]` → `- [x]`)
   - Update the Progress table status (`pending` → `in progress` or `done`)
2. **Run `go test ./...`** one final time to confirm everything passes.
3. **Commit all changes** with a clear message describing what was implemented. Stage specific files (not `git add -A`).

## Sub-Agent Guidelines

- Use `subagent_type: "general-purpose"` for all sub-agents
- Run sub-agents in **foreground** — you need each result before proceeding to the next step
- Provide **detailed, self-contained prompts** — sub-agents do not share your conversation context
- Include file paths, specs, code snippets, and patterns directly in each prompt
- If a sub-agent fails or produces incomplete results, you may re-launch it with a corrected prompt

## What to Build

Focus on working, tested endpoints that match the Mailgun API shape. Each endpoint should:
- Accept the correct HTTP method and path
- Parse the expected request format (JSON, multipart/form-data, or URL-encoded)
- Validate required fields and return appropriate errors
- Store/retrieve data via GORM models
- Return responses matching Mailgun's JSON structure (field names, casing, nesting)
- Handle pagination where specified in the plan doc

## Completion Signal

If you look at the implementation plan and **every task in every phase is checked off** (`- [x]`), output exactly:

```
ALL TODO ITEMS COMPLETE
```

Otherwise, do your work and commit. Do NOT output the completion signal unless literally everything is done.
