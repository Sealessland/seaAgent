#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
BIN_PATH="$ROOT_DIR/jetson_camera_agent"
LOG_PATH="/tmp/jetson_camera_agent.log"
GO_BIN="${GO_BIN:-$HOME/.local/toolchains/go1.23.12/bin/go}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

LISTEN_ADDR="${JETSON_AGENT_LISTEN_ADDR:?JETSON_AGENT_LISTEN_ADDR is required}"
BASE_URL="${OPENAI_BASE_URL:?OPENAI_BASE_URL is required}"
MODEL_NAME="${OPENAI_MODEL_NAME:?OPENAI_MODEL_NAME is required}"
API_KEY="${OPENAI_API_KEY:?OPENAI_API_KEY is required}"

build() {
  cd "$ROOT_DIR"
  GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  GOSUMDB="${GOSUMDB:-sum.golang.google.cn}" \
  "$GO_BIN" build -o "$BIN_PATH" ./cmd/jetson_camera_agent
}

stop() {
  pkill -f "$BIN_PATH" >/dev/null 2>&1 || true
}

start() {
  build
  stop
  cd "$ROOT_DIR"
  setsid -f env \
    JETSON_AGENT_LISTEN_ADDR="$LISTEN_ADDR" \
    OPENAI_BASE_URL="$BASE_URL" \
    OPENAI_MODEL_NAME="$MODEL_NAME" \
    OPENAI_API_KEY="$API_KEY" \
    "$BIN_PATH" >"$LOG_PATH" 2>&1 < /dev/null
  sleep 2
  status
}

status() {
  echo "listen: $LISTEN_ADDR"
  echo "base_url: $BASE_URL"
  echo "model: $MODEL_NAME"
  ps -ef | grep "$BIN_PATH" | grep -v grep || true
}

health() {
  curl -sS "http://$LISTEN_ADDR/api/health"
  echo
}

capture() {
  curl -sS "http://$LISTEN_ADDR/api/camera/capture"
  echo
}

preview_head() {
  curl -I -sS "http://$LISTEN_ADDR/api/camera/latest.jpg"
}

analyze() {
  local prompt="${1:-Describe the current camera view briefly.}"
  curl -sS -X POST "http://$LISTEN_ADDR/api/capture-and-analyze" \
    -H 'Content-Type: application/json' \
    -d "{\"prompt\":\"$prompt\"}"
  echo
}

logs() {
  tail -n 120 "$LOG_PATH"
}

case "${1:-}" in
  start)
    start
    ;;
  stop)
    stop
    ;;
  restart)
    stop
    start
    ;;
  build)
    build
    ;;
  status)
    status
    ;;
  health)
    health
    ;;
  capture)
    capture
    ;;
  preview-head)
    preview_head
    ;;
  analyze)
    shift
    analyze "${*:-}"
    ;;
  logs)
    logs
    ;;
  *)
    cat <<'EOF'
Usage:
  ./manage_jetson_camera_agent.sh start
  ./manage_jetson_camera_agent.sh stop
  ./manage_jetson_camera_agent.sh restart
  ./manage_jetson_camera_agent.sh build
  ./manage_jetson_camera_agent.sh status
  ./manage_jetson_camera_agent.sh health
  ./manage_jetson_camera_agent.sh capture
  ./manage_jetson_camera_agent.sh preview-head
  ./manage_jetson_camera_agent.sh analyze "Describe the current camera view briefly."
  ./manage_jetson_camera_agent.sh logs
EOF
    exit 1
    ;;
esac
