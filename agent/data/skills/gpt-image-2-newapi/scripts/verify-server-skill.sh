#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

expected_version="${EXPECTED_SKILL_VERSION:-2026-06-04-final-result-required}"

node --check scripts/generate-response-image.js >/dev/null

actual_version="$(node scripts/generate-response-image.js --version | sed -n 's/^script_version=//p' | tail -1)"
if [[ "$actual_version" != "$expected_version" ]]; then
  echo "version_ok=no expected=$expected_version actual=$actual_version"
  exit 1
fi
echo "version_ok=yes version=$actual_version"

node scripts/generate-response-image.js --check-config

if grep -q "partial_image=.*not a successful result" SKILL.md; then
  echo "partial_policy_ok=yes"
else
  echo "partial_policy_ok=no"
  exit 1
fi
