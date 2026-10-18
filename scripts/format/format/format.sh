#!/usr/bin/env bash
set -e

go mod tidy

# This runs in a job with write permission, so don't run dependency scripts
bun install --ignore-scripts

# Generate each AI agent's instruction file from the rules in .rulesync/
bun run rulesync generate
