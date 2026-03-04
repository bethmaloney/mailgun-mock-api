#!/bin/bash

# Script to repeatedly run Claude Code with TESTING_PROMPT.md
# Each iteration: orchestrator selects a test file, then spawns sub-agents for:
#   1. Test writing (implement test stubs, OK if tests fail)
#   2. Fixer (make failing tests pass by fixing server code)
#   3. Reviewer (audit tests + fixes, flag issues by severity)
#   Main agent fixes any CRITICAL/HIGH issues from reviewer
# Exits early if "ALL TODO ITEMS COMPLETE" is detected

set -e

PROMPT_FILE="scripts/TESTING_PROMPT.md"
MAX_ITERATIONS=30
ITERATION=0
CURRENT_BRANCH=$(git branch --show-current)

# Function to push with rebase and conflict resolution
push_with_rebase() {
    local max_rebase_retries=3
    local rebase_retry=0

    while [ $rebase_retry -lt $max_rebase_retries ]; do
        # Try to push
        if git push origin "$CURRENT_BRANCH" 2>/dev/null; then
            return 0
        fi

        # Push failed, try to set upstream if needed
        if git push -u origin "$CURRENT_BRANCH" 2>/dev/null; then
            return 0
        fi

        rebase_retry=$((rebase_retry + 1))
        echo -e "\nPush failed (attempt $rebase_retry of $max_rebase_retries). Trying pull --rebase..."

        # Fetch latest
        git fetch origin "$CURRENT_BRANCH"

        # Try rebase
        if git pull --rebase origin "$CURRENT_BRANCH" 2>&1; then
            echo "Rebase successful, retrying push..."
            continue
        fi

        # Check if there are conflicts
        if git status | grep -q "Unmerged paths\|both modified\|both added"; then
            echo -e "\nMerge conflicts detected. Launching Claude to resolve..."

            CONFLICT_FILES=$(git diff --name-only --diff-filter=U)
            CONFLICT_STATUS=$(git status)

            echo "Resolve merge conflicts. We are rebasing our local commits onto the updated remote.

IMPORTANT: In rebase conflicts, HEAD/ours = remote changes, incoming/theirs = our local work.
We want to KEEP OUR LOCAL WORK while incorporating any non-conflicting remote updates.

Conflicting files:
$CONFLICT_FILES

For each file:
1. Read the file to see the conflict markers
2. Decide how to merge the changes, ensuring we keep our local work
3. Run \`git add <file>\`

After all conflicts resolved: \`git rebase --continue\`" | claude -p \
                --dangerously-skip-permissions \
                --output-format=stream-json \
                --model sonnet \
                --verbose \
                2>&1 | tee >(claude-stream-format > /dev/stderr) > /dev/null

            # Check if rebase completed
            if git status | grep -q "rebase in progress"; then
                echo "Rebase still in progress after conflict resolution attempt. Aborting rebase..."
                git rebase --abort
                return 1
            fi
        else
            # Some other rebase error
            echo "Rebase failed with non-conflict error. Aborting..."
            git rebase --abort 2>/dev/null || true
            return 1
        fi
    done

    echo "Failed to push after $max_rebase_retries attempts"
    return 1
}

# Ensure prompt file exists
if [ ! -f "$PROMPT_FILE" ]; then
    echo "Error: Prompt file not found: $PROMPT_FILE"
    exit 1
fi

echo "Starting integration testing loop on branch: $CURRENT_BRANCH"
echo "Max iterations: $MAX_ITERATIONS"
echo "Prompt file: $PROMPT_FILE"
echo "Workflow: orchestrator → test writer → fixer (if needed) → reviewer → fix critical/high"
echo ""

while true; do
    ITERATION=$((ITERATION + 1))
    echo -e "\n======================== ITERATION $ITERATION of $MAX_ITERATIONS ========================\n"

    if [ $ITERATION -gt $MAX_ITERATIONS ]; then
        echo "Reached max iterations: $MAX_ITERATIONS"
        break
    fi

    # Run Claude Code orchestrator iteration
    OUTPUT=$(cat "$PROMPT_FILE" | claude -p \
        --dangerously-skip-permissions \
        --output-format=stream-json \
        --model opus \
        --verbose \
        2>&1 | tee >(claude-stream-format > /dev/stderr))

    # Check for completion signal
    if echo "$OUTPUT" | grep -q "ALL TODO ITEMS COMPLETE"; then
        echo -e "\n======================== SUCCESS ========================"
        echo "Detected 'ALL TODO ITEMS COMPLETE' - exiting loop"

        # Final push
        push_with_rebase

        echo "All done!"
        exit 0
    fi

    # Run lint check as safety net
    echo -e "\n------------------------ LINT CHECK ------------------------"
    LINT_RETRIES=0
    MAX_LINT_RETRIES=3
    LINT_PASSED=false

    if LINT_OUTPUT=$(just lint 2>&1); then
        LINT_PASSED=true
        echo "$LINT_OUTPUT"
    else
        echo "$LINT_OUTPUT"
        while [ $LINT_RETRIES -lt $MAX_LINT_RETRIES ]; do
            LINT_RETRIES=$((LINT_RETRIES + 1))
            echo -e "\nLint failed (attempt $LINT_RETRIES of $MAX_LINT_RETRIES)"
            echo -e "\nLaunching Claude to fix lint errors..."

            echo "Fix all lint errors (Go vet and ESLint). Here is the output from \`just lint\`:

\`\`\`
$LINT_OUTPUT
\`\`\`

Run \`just lint\` to verify fixes. Commit any changes with an appropriate message." | claude -p \
                --dangerously-skip-permissions \
                --output-format=stream-json \
                --model sonnet \
                --verbose \
                2>&1 | tee >(claude-stream-format > /dev/stderr) > /dev/null

            echo -e "\nRetrying lint..."
            if LINT_OUTPUT=$(just lint 2>&1); then
                LINT_PASSED=true
                break
            else
                echo "$LINT_OUTPUT"
            fi
        done
    fi

    if [ "$LINT_PASSED" = true ]; then
        echo -e "------------------------ LINT PASSED ------------------------\n"
    else
        echo -e "------------------------ LINT FAILED (continuing anyway) ------------------------\n"
    fi

    # Push changes after each iteration
    echo -e "\nPushing changes..."
    push_with_rebase

    echo -e "\nIteration $ITERATION complete. Continuing..."
done

echo "Loop finished without completion signal."
exit 1
