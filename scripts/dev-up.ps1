$ErrorActionPreference = "Stop"

$composeArgs = @("-p", "note-app-local-db", "-f", "docker-compose.local-db.yml")

foreach ($legacyName in @("note-app-local-postgres", "note-app-postgres")) {
    $legacyContainer = (docker ps -aq -f "name=^${legacyName}$")
    if ($legacyContainer) {
        docker rm -f $legacyName | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to remove legacy container $legacyName"
        }
    }
}

docker compose @composeArgs up -d --force-recreate postgres
if ($LASTEXITCODE -ne 0) {
    throw "failed to start local postgres via docker compose"
}

$containerId = (docker compose @composeArgs ps -q postgres).Trim()
if (-not $containerId) {
    throw "postgres container was not created"
}

for ($i = 0; $i -lt 30; $i++) {
    $status = (docker inspect --format "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}" $containerId).Trim()
    if ($status -eq "healthy") {
        break
    }
    Start-Sleep -Seconds 2
}

$finalStatus = (docker inspect --format "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}" $containerId).Trim()
if ($finalStatus -ne "healthy") {
    throw "postgres container is not healthy (status: $finalStatus)"
}

go run ./cmd/migrate -env-file .env.local -direction up
if ($LASTEXITCODE -ne 0) {
    throw "failed to run development migrations"
}

go run ./cmd/api -env-file .env.local
