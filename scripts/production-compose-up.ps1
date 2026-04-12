$ErrorActionPreference = "Stop"

docker compose --env-file .env.production up -d --build
