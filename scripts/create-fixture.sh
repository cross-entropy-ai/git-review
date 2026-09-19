#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
fixture="${1:-$root/.review-fixture}"
if [[ -e "$fixture" ]]; then
  echo "Fixture already exists: $fixture (kept as-is)"
  exit 0
fi

mkdir -p "$fixture"
git init -q -b main "$fixture"
git -C "$fixture" config user.name 'Git Review Demo'
git -C "$fixture" config user.email 'demo@git-review.local'
git -C "$fixture" config commit.gpgsign false
git -C "$fixture" config core.hooksPath /dev/null
mkdir -p "$fixture/internal/server" "$fixture/docs" "$fixture/assets"
cat > "$fixture/internal/server/server.go" <<'EOF'
package server

import (
	"fmt"
	"net/http"
)

// Handler serves the application health check.
func Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "OK")
}
EOF
cat > "$fixture/config.json" <<'EOF'
{
  "port": 8080,
  "timeout": 10,
  "endpoint": "http://localhost:8080/health",
  "logging": false
}
EOF
printf '# Getting started\n\nRun the server locally.\n' > "$fixture/docs/start.md"
printf 'Legacy configuration\nNo longer needed\n' > "$fixture/legacy.txt"
printf '\x89PNG\x00old-demo\n' > "$fixture/assets/logo.png"
git -C "$fixture" add .
git -C "$fixture" commit -qm 'Initial demo service'
git -C "$fixture" switch -qc feature/health-check
cat > "$fixture/internal/server/server.go" <<'EOF'
package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// Handler reports structured health information for probes.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	response := map[string]any{
		"status": "healthy",
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}
EOF
cat > "$fixture/config.json" <<'EOF'
{
  "port": 9090,
  "timeout": 30,
  "endpoint": "https://localhost:9090/ready",
  "logging": true,
  "health_path": "/health"
}
EOF
git -C "$fixture" mv docs/start.md 'docs/quick start.md'
printf '\nHealth checks return JSON from `/health`.\n' >> "$fixture/docs/quick start.md"
rm "$fixture/legacy.txt"
printf '\x89PNG\x00new-demo\n' > "$fixture/assets/logo.png"
printf 'export const healthPath = "/health";\nexport const retryCount = 3;\n' > "$fixture/client.ts"
printf '# Deployment notes\n\nUnicode paths and spaces are supported.\n' > "$fixture/docs/部署说明.md"
printf 'No final newline' > "$fixture/no-newline.txt"
git -C "$fixture" add .
git -C "$fixture" commit -qm 'Add structured health checks'
echo "Created $fixture"
echo "Run: just demo"
