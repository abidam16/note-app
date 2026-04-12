#!/usr/bin/env bash
set -euo pipefail

docker compose --env-file .env.production up -d --build
