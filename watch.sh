#!/usr/bin/env bash

# Calls `bun run build` each time playground.js is edited.

echo public/playground.js | entr -s 'bun run build'
