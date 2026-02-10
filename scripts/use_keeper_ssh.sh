#!/usr/bin/env bash
set -euo pipefail

# Helper to verify Keeper ssh-agent socket inside the container and test GitHub SSH auth.
# Usage: ./scripts/use_keeper_ssh.sh

SSH_AUTH_SOCK=${SSH_AUTH_SOCK:-/run/keeper-ssh-agent.sock}
export SSH_AUTH_SOCK

echo "Using SSH_AUTH_SOCK=$SSH_AUTH_SOCK"
if [ -S "$SSH_AUTH_SOCK" ]; then
  echo "Socket exists. Listing available public keys (ssh-add -L):"
  ssh-add -L || echo "ssh-add failed (no identities)"
else
  echo "Socket not found: $SSH_AUTH_SOCK"
  exit 1
fi

echo
echo "Testing SSH to GitHub (may fail if keys aren't authorized)"
ssh -T git@github.com || true
