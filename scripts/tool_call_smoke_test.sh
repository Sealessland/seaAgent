#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-18080}"
TOKEN="${1:-smoke-001}"
PROMPT="${2:-/tooltest 请调用 tool_call_smoke_test，token 使用 ${TOKEN}，并根据工具结果简短回复。}"

curl -sS "http://127.0.0.1:${PORT}/api/agent/chat" \
  -H 'Content-Type: application/json' \
  --data-binary @- <<EOF
{"message":"${PROMPT}","use_latest_image":false,"include_snapshot":false}
EOF
