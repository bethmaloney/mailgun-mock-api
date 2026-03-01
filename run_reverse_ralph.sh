#!/bin/bash

# Script to repeatedly run Claude Code to flesh out implementation plan docs
# Exits early if "ALL PLAN DOCS COMPLETE" is detected

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STREAM_FORMAT="$SCRIPT_DIR/scripts/claude-stream-format.py"
PROMPT_FILE="prompt_reverse_engineer.md"
MAX_ITERATIONS=40
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
                2>&1 | tee >(python3 "$STREAM_FORMAT" > /dev/stderr) > /dev/null

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

echo "Starting planning loop on branch: $CURRENT_BRANCH"
echo "Max iterations: $MAX_ITERATIONS"
echo "Prompt file: $PROMPT_FILE"
echo ""

while true; do
    ITERATION=$((ITERATION + 1))
    echo -e "\n======================== ITERATION $ITERATION of $MAX_ITERATIONS ========================\n"

    if [ $ITERATION -gt $MAX_ITERATIONS ]; then
        echo "Reached max iterations: $MAX_ITERATIONS"
        break
    fi

    # Run Claude Code iteration
    OUTPUT=$(cat "$PROMPT_FILE" | claude -p \
        --dangerously-skip-permissions \
        --output-format=stream-json \
        --model opus \
        --verbose \
        2>&1 | tee >(python3 "$STREAM_FORMAT" > /dev/stderr))

    # Check for completion signal
    if echo "$OUTPUT" | grep -q "ALL PLAN DOCS COMPLETE"; then
        echo -e "\n======================== SUCCESS ========================"
        echo "Detected 'ALL PLAN DOCS COMPLETE' - exiting loop"

        # Final push
        push_with_rebase

        echo "All done!"
        exit 0
    fi

    # Push changes after each iteration
    echo -e "\nPushing changes..."
    push_with_rebase

    echo -e "\nIteration $ITERATION complete. Continuing..."
done

echo "Loop finished without completion signal."
exit 1
