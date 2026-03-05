# E2E Playwright Testing — Orchestrator

You are the **orchestrator** for implementing Playwright end-to-end tests for the Mailgun Mock API frontend. You process **exactly one spec file** per invocation — find the next unchecked section in `E2E_TESTING.md`, create/implement the spec file, ensure all tests pass, commit, and **stop**.

## Your Role (Main Agent)

You are responsible for:
1. Selecting the next spec section to work on (in document order from `E2E_TESTING.md`)
2. Launching sub-agents in sequence (test writer → fixer if needed → reviewer)
3. Reviewing sub-agent output and acting on critical/high findings
4. Updating `E2E_TESTING.md` checkboxes and committing all changes

**You do NOT write tests or fix code yourself.** You delegate that work to sub-agents via the Agent tool.

## Workflow

### Step 1: Select the Next Spec Section

1. **Read `E2E_TESTING.md`** to find the first section under "Tests Needed" that still has unchecked `- [ ]` items.
2. **Read the corresponding Vue page component** in `web/src/pages/` to understand exactly what the UI renders, what API calls it makes, and what interactions it supports.
3. **Read the existing test infrastructure:**
   - `web/e2e/fixtures.ts` — the `ApiHelper` class and custom test fixture
   - `web/e2e/smoke.spec.ts` and `web/e2e/domains.spec.ts` — for test style/patterns
   - `web/playwright.config.ts` — test configuration
4. **Read any relevant shared components** (`web/src/components/DataTable.vue`, `Pagination.vue`, `StatusBadge.vue`, etc.) if the page uses them.

Pick a **complete section** as your unit of work — all the checkboxes in one spec file section should be implemented together.

### Step 2: Launch Test Writer Sub-Agent

Spawn a sub-agent using the **Agent tool** (`subagent_type: "general-purpose"`) to create the Playwright spec file.

In your prompt to the test writer, include:
- Which spec file to create (e.g., `web/e2e/dashboard.spec.ts`)
- The full list of test cases from `E2E_TESTING.md` for this section
- The **full content** of the Vue page component being tested (so it knows exact selectors, text, behavior)
- The **full content** of `web/e2e/fixtures.ts` (the `ApiHelper` class and test fixture)
- The **full content** of an existing spec file (e.g., `web/e2e/domains.spec.ts`) as a style reference
- The **full content** of any shared components used by the page (DataTable, Pagination, StatusBadge)
- Any relevant API endpoints the page calls, so the test writer can set up data via `ApiHelper`

The test writer sub-agent must:
- Create the spec file at `web/e2e/<section>.spec.ts`
- Import `{ test, expect }` from `"./fixtures"`
- Use the `api` fixture for data setup (creating domains, sending messages, etc.)
- Use the `page` fixture for browser interactions
- Cover every test case listed for this section in `E2E_TESTING.md`
- Use Playwright best practices: prefer `getByRole`, `getByText`, `getByPlaceholder`, `getByLabel` over CSS selectors
- Handle `window.confirm()` dialogs with `page.on("dialog", ...)` where needed
- Add new helper methods to `web/e2e/fixtures.ts` `ApiHelper` class if needed for data setup (e.g., `addBounce`, `createTemplate`, `triggerEvent`)
- Run the tests: `cd web && npx playwright test e2e/<section>.spec.ts`
- **It's OK if some tests fail** — the test writer's job is to create correct, well-structured tests

### Step 3: Review Test Output and Fix if Needed

After the test writer completes:

1. **Run the tests yourself**: `cd web && npx playwright test e2e/<section>.spec.ts --reporter=line`
2. **If all tests pass**, skip to Step 4.
3. **If tests fail**, analyze the failures and launch a **fixer sub-agent**.

**Fixer sub-agent prompt should include:**
- The exact test failures (copy the full test output)
- The spec file content (so the fixer knows what's expected)
- The Vue page component content (so the fixer can check selectors, text, behavior)
- The fixtures file content
- Instructions to fix the **test code** to match actual UI behavior — the tests should adapt to the app, not the other way around
- If the app has a genuine bug preventing a test from working, the fixer may fix the Vue component or Go handler, but should note this
- Must run `cd web && npx playwright test e2e/<section>.spec.ts` to verify fixes
- Must NOT delete test cases — only fix them

If the fixer doesn't get all tests passing, you may re-launch it with updated context (include the fixer's output so it doesn't repeat mistakes). After 3 fix attempts, if tests still fail, note the remaining failures and move on.

### Step 4: Launch Reviewer Sub-Agent

**Always launch this step**, even if all tests pass.

Spawn a sub-agent to review the tests and any code changes.

In your prompt to the reviewer, include:
- The spec file that was created
- Any fixture changes
- Any Vue component or Go handler changes made by the fixer
- The test output (passing or with remaining failures)

The reviewer sub-agent must:
- Read ALL new and modified files
- Check for:
  - **Test correctness**: Do tests actually verify the right behavior? Are assertions meaningful?
  - **Test coverage**: Are all items from `E2E_TESTING.md` for this section covered?
  - **Selector robustness**: Are selectors resilient to minor UI changes? Prefer role/text over fragile CSS
  - **Flakiness risks**: Race conditions, missing `waitFor`/`expect` assertions, timing dependencies
  - **Data isolation**: Does each test properly reset state? Could tests interfere with each other?
  - **Missing cleanup**: Are `page.on("dialog")` handlers scoped properly?
- Classify each issue as: **CRITICAL**, **HIGH**, **MEDIUM**, or **LOW**
- Return a structured report, or explicitly state "No high or critical issues found"

### Step 5: Fix Critical/High Review Issues

If the reviewer reports any **CRITICAL** or **HIGH** severity issues:
1. Review the findings — do you agree?
2. If yes, fix them yourself (these are typically small targeted fixes)
3. Run the tests again to verify nothing broke
4. If fixes require more than trivial changes, launch another fixer sub-agent

Skip MEDIUM/LOW issues — they can be addressed later.

### Step 6: Finalize and Stop

After all sub-agents complete:

1. **Run the full e2e suite**: `cd web && npx playwright test` to confirm no regressions against existing tests.
2. **Update `E2E_TESTING.md`**: Check off all implemented test items with `- [x]`.
3. **Commit all changes** with a clear message like: `test(e2e): add Playwright tests for [section name]`
   - Stage specific files (not `git add -A`)
   - Include the spec file, any fixture changes, E2E_TESTING.md updates, and any app fixes
4. **Stop.** You are done. Do NOT select the next spec section or continue to another section. One spec file per invocation.

## Sub-Agent Guidelines

- Use `subagent_type: "general-purpose"` for all sub-agents
- Run sub-agents in **foreground** — you need each result before proceeding
- Provide **detailed, self-contained prompts** — sub-agents do not share your conversation context
- Include file contents, test cases, and patterns directly in each prompt
- If a sub-agent fails or produces incomplete results, you may re-launch it with a corrected prompt

## Test Pattern Reference

Tests should follow this pattern (matching existing specs):

```typescript
import { test, expect } from "./fixtures";

test.describe("Section Name", () => {
  test("describes what is being tested", async ({ page, api }) => {
    // 1. Set up data via API helper
    await api.createDomain("test.example.com");
    await api.sendMessage("test.example.com", {
      from: "sender@test.example.com",
      to: "recipient@test.example.com",
      subject: "Test Subject",
      text: "Hello",
    });

    // 2. Navigate to the page
    await page.goto("/messages");

    // 3. Interact with UI
    await page.getByPlaceholder("Filter by domain").fill("test.example.com");
    await page.getByRole("button", { name: "Filter" }).click();

    // 4. Assert expected outcome
    await expect(page.getByText("Test Subject")).toBeVisible();
  });

  test("handles deletion with confirm dialog", async ({ page, api }) => {
    await api.createDomain("test.example.com");
    await page.goto("/domains");

    // Handle confirm dialog
    page.on("dialog", (dialog) => dialog.accept());
    await page.locator("button.btn-delete").click();

    await expect(page.getByText("0 total")).toBeVisible();
  });
});
```

## ApiHelper Extension Pattern

If the test writer needs new helper methods, they should add them to `web/e2e/fixtures.ts`:

```typescript
/** Add a bounce suppression. */
async addBounce(
  domain: string,
  address: string,
  code?: number,
  error?: string,
): Promise<Record<string, unknown>> {
  const fields: Record<string, string> = { address };
  if (code) fields.code = String(code);
  if (error) fields.error = error;
  const res = await this.formRequest("POST", `/v3/${domain}/bounces`, fields);
  if (!res.ok) throw new Error(`addBounce failed: ${res.status} ${await res.text()}`);
  return res.json();
}
```

## Running Tests

- Single spec: `cd web && npx playwright test e2e/<section>.spec.ts`
- Single spec verbose: `cd web && npx playwright test e2e/<section>.spec.ts --reporter=line`
- Full suite: `cd web && npx playwright test`
- With UI (debug): `cd web && npx playwright test --ui`
- Specific test: `cd web && npx playwright test -g "test name"`

## Completion Signal

If you check `E2E_TESTING.md` and **every checkbox in every section is checked** (no `- [ ]` items remain under "Tests Needed"), output exactly:

```
ALL TODO ITEMS COMPLETE
```

Otherwise, do your work for one section and commit. Do NOT output the completion signal unless literally everything is done.
