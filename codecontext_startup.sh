#! /bin/bash

set -eu

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
LOG_DIR="$SCRIPT_DIR/log"
LOG_FILE="$LOG_DIR/codecontext.log"
GRAPH_DB="$SCRIPT_DIR/.codecontext.db"

if command -v uv >/dev/null 2>&1; then
	RUNNER=(uv run python -m codecontext)
else
	RUNNER=(python3 -m codecontext)
fi

mkdir -p "$LOG_DIR"

# If any existing MCP HTTP processes are running, stop them gracefully,
# wait a short time, then force-kill if still running.
if pgrep -f "python -m codecontext .*mcp -http|codecontext mcp -http" >/dev/null 2>&1; then
	echo "$(date --iso-8601=seconds) - Stopping existing codecontext mcp -http processes" >> "$LOG_FILE"
	pkill -f "python -m codecontext .*mcp -http|codecontext mcp -http" || true

	# wait up to 10s for processes to exit
	for i in $(seq 1 10); do
		if ! pgrep -f "python -m codecontext .*mcp -http|codecontext mcp -http" >/dev/null 2>&1; then
			break
		fi
		sleep 1
	done

	if pgrep -f "python -m codecontext .*mcp -http|codecontext mcp -http" >/dev/null 2>&1; then
		echo "$(date --iso-8601=seconds) - Forcing kill of remaining processes" >> "$LOG_FILE"
		pkill -9 -f "python -m codecontext .*mcp -http|codecontext mcp -http" || true
	fi
fi

echo "$(date --iso-8601=seconds) - Starting codecontext mcp -http" >> "$LOG_FILE"
nohup "${RUNNER[@]}" -graph "$GRAPH_DB" mcp -http >> "$LOG_FILE" 2>&1 &