#!/usr/bin/env bash
set -euo pipefail

# Requires: GitHub CLI (gh) authenticated and repo context.

declare -A LABELS=(
  ["type:feature"]="#3fb950"
  ["type:bug"]="#d73a4a"
  ["type:task"]="#c5def5"
  ["type:docs"]="#0366d6"
  ["type:infra"]="#fbca04"
  ["area:backend"]="#0e8a16"
  ["area:sync"]="#b60205"
  ["area:web"]="#1f6feb"
  ["area:native"]="#a2eeef"
  ["area:security"]="#e99695"
  ["area:ci"]="#5319e7"
  ["priority:P1"]="#b60205"
  ["priority:P2"]="#d876e3"
  ["good-first-issue"]="#7057ff"
  ["blocked"]="#eeeeee"
  ["design-needed"]="#bfdadc"
)

declare -A DESCRIPTIONS=(
  ["type:feature"]="New functionality or enhancement"
  ["type:bug"]="Something isn't working correctly"
  ["type:task"]="General maintenance or chore work"
  ["type:docs"]="Documentation improvements or additions"
  ["type:infra"]="Infrastructure, build, or deployment changes"
  ["area:backend"]="Server-side logic and API"
  ["area:sync"]="CalDAV synchronization and backend integration"
  ["area:web"]="Web interface and htmx frontend"
  ["area:native"]="Platform-native clients (desktop/mobile)"
  ["area:security"]="Security-related changes or concerns"
  ["area:ci"]="Continuous integration and testing"
  ["priority:P1"]="High priority, should be addressed soon"
  ["priority:P2"]="Medium priority, normal queue"
  ["good-first-issue"]="Good for newcomers to the project"
  ["blocked"]="Waiting on dependencies or external factors"
  ["design-needed"]="Requires design discussion or specification"
)

for name in "${!LABELS[@]}"; do
  color=${LABELS[$name]#\#}
  description="${DESCRIPTIONS[$name]}"
  if gh label list --json name | jq -e ".[].name == \"$name\"" >/dev/null 2>&1; then
    echo "Label exists: $name"
  else
    gh label create "$name" --color "$color" --description "$description" || echo "Failed to create $name"
  fi
done

echo "Labels bootstrap complete."
