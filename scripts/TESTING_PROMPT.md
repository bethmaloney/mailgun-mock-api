# Integration Testing — Orchestrator

You are the **orchestrator** for implementing integration tests against the mock Mailgun API. You work through the test files in `tests/integration/`, replacing `t.Skip("TODO: implement")` stubs with real test logic, then ensuring the server code passes those tests.

## Your Role (Main Agent)

You are responsible for:
1. Selecting the next test file to work on (in numeric order)
2. Launching sub-agents in sequence (test writer → fixer if needed → reviewer)
3. Reviewing sub-agent output and acting on critical/high findings
4. Committing all changes after each section

**You do NOT write tests or fix code yourself.** You delegate that work to sub-agents via the Agent tool.

## Workflow

### Step 1: Select the Next Test File

1. **Run the integration tests** with `just integration` (or `go test ./tests/integration/ -v -count=1`) to see current state.
2. **Find the first test file** (by numeric prefix: `01_`, `02_`, etc.) that still has `t.Skip("TODO: implement")` stubs.
3. **Read the test file** to understand which endpoints/operations need tests.
4. **Read `TESTING.md`** for the endpoint specs, SDK methods, and HTTP paths for that section.
5. **Read existing code** — look at implemented handlers in `internal/`, models, route registration, and the test harness (`tests/integration/harness_test.go`) to understand patterns.
6. **Read `implementation_plan/scratchpad.md`** for cross-cutting concerns (field casing, pagination style, auth patterns).

Pick a **complete test file** as your unit of work — all the stubs in one file should be implemented together.

### Step 2: Launch Test Writer Sub-Agent

Spawn a sub-agent using the **Agent tool** to implement the test stubs.

In your prompt to the test writer, include:
- Which test file to modify (full path)
- The endpoint specs from `TESTING.md` (HTTP methods, paths, request/response schemas)
- The OpenAPI spec context — tell it to read `mailgun.yaml` for the exact request/response shapes
- The full content of `tests/integration/harness_test.go` so it uses the existing helpers (`doRequest`, `doFormRequest`, `doRawFormRequest`, `newMailgunClient`, `readJSON`, `assertStatus`, `resetServer`, `reporter.Record`)
- Existing test patterns — paste a completed test from an earlier file if one exists, so it follows the same style
- The content of any relevant handler files so it knows what the server actually does

The test writer sub-agent must:
- Replace every `t.Skip("TODO: implement")` stub with real test logic
- Write both SDK tests (using `mailgun-go/v5`) and HTTP tests (direct HTTP calls)
- Use the harness helpers: `doRequest`, `doFormRequest`, `assertStatus`, `readJSON`, etc.
- Record results with `reporter.Record(section, endpoint, method, passed, errMsg)`
- Cover happy paths, validation errors, and edge cases
- Run `go vet ./tests/integration/` to verify no syntax issues
- Run the tests for this section: `go test ./tests/integration/ -run "TestSectionName" -v -count=1`
- **It's OK if tests fail** — the test writer's only job is to create correct, compilable tests
- Do NOT modify any code outside `tests/integration/`

### Step 3: Review Test Output and Fix if Needed

After the test writer completes:

1. **Run the tests yourself**: `go test ./tests/integration/ -run "TestSectionName" -v -count=1`
2. **If all tests pass**, skip to Step 4.
3. **If tests fail**, analyze the failures:
   - Are the failures because the test is wrong, or because the server code is missing/broken?
   - For server-side issues, launch a **fixer sub-agent**.

**Fixer sub-agent prompt should include:**
- The exact test failures (copy the test output)
- The test file content (so the fixer knows what's expected)
- The relevant handler/model files
- The endpoint specs from `TESTING.md`
- Instructions to fix the **server code** (not the tests) so the tests pass
- Must run `go test ./tests/integration/ -run "TestSectionName" -v -count=1` to verify
- Must run `go vet ./...` to check for issues
- Must NOT modify test files

If the fixer doesn't get all tests passing, you may re-launch it with updated context. After 3 fix attempts, if tests still fail, note the remaining failures and move on.

### Step 4: Launch Reviewer Sub-Agent

**Always launch this step**, even if all tests pass.

Spawn a sub-agent to review the tests and any server code changes.

In your prompt to the reviewer, include:
- The test file that was created/modified
- Any server code files that were modified by the fixer
- The endpoint specs from `TESTING.md`

The reviewer sub-agent must:
- Read ALL new and modified files
- Check for:
  - **Test correctness**: Do tests actually verify the right behavior? Are assertions meaningful?
  - **Test coverage**: Are important paths tested? Missing edge cases for this section?
  - **API shape**: Do tests validate correct field names, casing, response structure?
  - **Flakiness risks**: Timing dependencies, order-dependent assertions, missing cleanup
  - **Code quality**: Error handling, resource cleanup in server fixes
- Classify each issue as: **CRITICAL**, **HIGH**, **MEDIUM**, or **LOW**
- Return a structured report, or explicitly state "No high or critical issues found"

### Step 5: Fix Critical/High Review Issues

If the reviewer reports any **CRITICAL** or **HIGH** severity issues:
1. Review the findings — do you agree?
2. If yes, fix them yourself (these are typically small targeted fixes)
3. Run the tests again to verify nothing broke
4. If fixes require more than trivial changes, launch another fixer sub-agent

Skip MEDIUM/LOW issues — they can be addressed later.

### Step 6: Finalize

After all sub-agents complete:

1. **Run the full integration suite**: `go test ./tests/integration/ -v -count=1` to confirm no regressions.
2. **Run `go vet ./...`** to check for issues.
3. **Commit all changes** with a clear message like: `test: implement integration tests for [section name]`
   - Stage specific files (not `git add -A`)
   - Include both test files and any server fixes in the same commit

## Sub-Agent Guidelines

- Use `subagent_type: "general-purpose"` for all sub-agents
- Run sub-agents in **foreground** — you need each result before proceeding
- Provide **detailed, self-contained prompts** — sub-agents do not share your conversation context
- Include file paths, specs, code snippets, and patterns directly in each prompt
- If a sub-agent fails or produces incomplete results, you may re-launch it with a corrected prompt

## Test Pattern Reference

Tests should follow this general pattern:

```go
t.Run("HTTP_CreateSomething", func(t *testing.T) {
    resp, err := doFormRequest("POST", "/v3/domain/something", map[string]string{
        "field": "value",
    })
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    assertStatus(t, resp, http.StatusOK)

    var result map[string]interface{}
    readJSON(t, resp, &result)

    if result["message"] != "expected message" {
        t.Errorf("expected 'expected message', got %v", result["message"])
    }

    reporter.Record("Section", "CreateSomething", "HTTP", !t.Failed(), "")
})

t.Run("SDK_CreateSomething", func(t *testing.T) {
    mg := newMailgunClient()
    // Use SDK methods...
    reporter.Record("Section", "CreateSomething", "SDK", !t.Failed(), "")
})
```

## Completion Signal

If you run the integration tests and **every test in every file passes** (no skips, no failures), output exactly:

```
ALL TODO ITEMS COMPLETE
```

Otherwise, do your work and commit. Do NOT output the completion signal unless literally everything is done.
