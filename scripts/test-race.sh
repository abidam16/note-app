#!/usr/bin/env bash
set -euo pipefail

compose_args=(-p note-app-local-db -f docker-compose.local-db.yml)

docker compose "${compose_args[@]}" up -d --force-recreate postgres

container_id="$(docker compose "${compose_args[@]}" ps -q postgres | tr -d '\r')"
if [[ -z "${container_id}" ]]; then
  echo "postgres container was not created" >&2
  exit 1
fi

for _ in {1..30}; do
  status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container_id}" | tr -d '\r')"
  if [[ "${status}" == "healthy" ]]; then
    break
  fi
  sleep 2
done

final_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container_id}" | tr -d '\r')"
if [[ "${final_status}" != "healthy" ]]; then
  echo "postgres container is not healthy (status: ${final_status})" >&2
  exit 1
fi

go run ./cmd/migrate -env-file .env.test -direction up

TEST_ENV_FILE=.env.test \
CC="${CC:-gcc}" \
CXX="${CXX:-g++}" \
go test -race -p 1 ./internal/application ./internal/repository/postgres ./internal/transport/http "$@"
