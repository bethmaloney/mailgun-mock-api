#!/usr/bin/env python3
"""Format Claude Code's stream-json output into human-readable text.

Port of the Rust claude-stream-format tool. Reads NDJSON from stdin,
prints emoji-prefixed summaries for assistant messages and tool calls.
"""

import json
import sys


def truncate(s, max_len=80):
    if s is None:
        return ""
    s = str(s)
    if len(s) <= max_len:
        return s
    return s[: max_len - 3] + "..."


def format_tool_use(name, inp):
    if name == "Read":
        return f"📖 Read: {inp.get('file_path', '?')}"
    if name == "Edit":
        return f"✏️  Edit: {inp.get('file_path', '?')}"
    if name == "Write":
        return f"📝 Write: {inp.get('file_path', '?')}"
    if name == "Bash":
        return f"💻 Bash: {truncate(inp.get('command', '?'))}"
    if name == "Glob":
        return f"🔍 Glob: {inp.get('pattern', '?')}"
    if name == "Grep":
        return f"🔍 Grep: {inp.get('pattern', '?')}"
    if name == "TodoWrite":
        todos = inp.get("todos", [])
        if todos:
            items = [t.get("content", t.get("subject", "")) for t in todos[:3] if isinstance(t, dict)]
            summary = ", ".join(i for i in items if i)
            return f"📋 TodoWrite: {truncate(summary)}" if summary else "📋 TodoWrite"
        return "📋 TodoWrite"
    if name == "TaskCreate":
        return f"📋 TaskCreate: {truncate(inp.get('subject', '?'))}"
    if name == "TaskUpdate":
        status = inp.get("status", "")
        task_id = inp.get("taskId", "?")
        return f"📋 TaskUpdate: #{task_id} → {status}" if status else f"📋 TaskUpdate: #{task_id}"
    if name == "TaskList":
        return "📋 TaskList"
    if name == "Agent":
        desc = inp.get("description", "?")
        agent_type = inp.get("subagent_type", "")
        return f"🤖 Agent ({agent_type}): {desc}" if agent_type else f"🤖 Agent: {desc}"
    if name == "Skill":
        return f"⚡ Skill: {inp.get('skill', '?')}"
    return f"🔧 {name}"


def process_line(line):
    try:
        msg = json.loads(line)
    except (json.JSONDecodeError, ValueError):
        return None

    msg_type = msg.get("type")

    if msg_type == "assistant":
        message = msg.get("message")
        if not message:
            return None

        output = []
        for block in message.get("content", []):
            block_type = block.get("type")
            if block_type == "text":
                text = block.get("text", "").strip()
                if text:
                    output.append(text)
            elif block_type == "tool_use":
                output.append(format_tool_use(block.get("name", "?"), block.get("input", {})))

        return "\n".join(output) if output else None

    if msg_type == "result":
        result = msg.get("result")
        if result:
            return f"✅ Done: {truncate(result)}"

    return None


def main():
    for line in sys.stdin:
        line = line.rstrip("\n")
        if not line:
            continue
        output = process_line(line)
        if output:
            print(output, flush=True)


if __name__ == "__main__":
    main()
