#!/usr/bin/env bash
# Prints {app: {component: [env keys]}} for both production templates as one line of JSON.
set -euo pipefail
settings() {
  yq -o=json -I=0 '[.services[]?, .jobs[]?] | map({"key": .name, "value": [.envs[]?.key]}) | from_entries' "$1"
}
printf '{"coti-core":%s,"coti-portal":%s}\n' "$(settings .do/app.neon.yaml)" "$(settings .do/portal.yaml)"
