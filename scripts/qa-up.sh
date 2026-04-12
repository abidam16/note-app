#!/usr/bin/env bash
set -euo pipefail

compose_args=(-p note-app-local-db -f docker-compose.local-db.yml)

for legacy_name in note-app-local-postgres note-app-postgres; do
  if docker ps -aq -f "name=^${legacy_name}$" | grep -q .; then
    docker rm -f "${legacy_name}" >/dev/null
  fi
done

docker compose "${compose_args[@]}" up -d --force-recreate postgres
container_id="$(docker compose "${compose_args[@]}" ps -q postgres)"
if [[ -z "${container_id}" ]]; then
  echo "postgres container was not created" >&2
  exit 1
fi

for _ in $(seq 1 30); do
  status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container_id}")"
  if [[ "${status}" == "healthy" ]]; then
    break
  fi
  sleep 2
done

final_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container_id}")"
if [[ "${final_status}" != "healthy" ]]; then
  echo "postgres container is not healthy (status: ${final_status})" >&2
  exit 1
fi

go run ./cmd/migrate -env-file .env.test -direction up
exec go run ./cmd/api -env-file .env.test
