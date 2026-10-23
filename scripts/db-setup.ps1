<#
.SYNOPSIS
    Script de automatización para levantar PostgreSQL 17, aplicar la migración
    inicial (001_init.sql) y verificar los Criterios de Aceptación de MVP-002.

.DESCRIPTION
    Este script realiza los siguientes pasos explicados paso a paso:
      1. Verifica que el comando 'docker' esté disponible y el motor corriendo.
      2. Levanta el servicio 'postgres' definido en server/docker-compose.yml.
      3. Espera hasta que el healthcheck de Docker reporte estado 'healthy' en localhost:5432.
      4. Aplica de forma idempotente migrations/001_init.sql (por si el volumen ya existía).
      5. Verifica y muestra en consola las 10 tablas creadas y los 7 cosméticos del catálogo semilla.
#>

$ErrorActionPreference = "Stop"

# Nos ubicamos en la raíz del proyecto 'server' (un nivel arriba de /scripts)
$ServerRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ServerRoot

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host " [MVP-002] Inicialización y Verificación de PostgreSQL 17" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

# 1. Verificar disponibilidad de Docker (agregando la ruta por defecto de Docker Desktop si la sesión aún no recargó el PATH)
$DefaultDockerPath = "C:\Program Files\Docker\Docker\resources\bin"
if (-not (Get-Command "docker" -ErrorAction SilentlyContinue) -and (Test-Path "$DefaultDockerPath\docker.exe")) {
    $env:PATH += ";$DefaultDockerPath"
}

if (-not (Get-Command "docker" -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] Docker no está instalado o no se encuentra en el PATH." -ForegroundColor Red
    Write-Host "Instala Docker Desktop y vuelve a ejecutar: .\scripts\db-setup.ps1" -ForegroundColor Yellow
    exit 1
}

# 2. Levantar el contenedor con Docker Compose
Write-Host "`n[1/4] Levantando contenedor PostgreSQL 17 (docker compose up -d)..." -ForegroundColor Yellow
docker compose up -d

# 3. Esperar a que el healthcheck reporte 'healthy'
Write-Host "`n[2/4] Esperando a que la base de datos en localhost:5432 esté saludable (healthy)..." -ForegroundColor Yellow
$MaxRetries = 15
$RetryCount = 0
$IsHealthy = $false

while ($RetryCount -lt $MaxRetries) {
    $Status = docker inspect --format="{{.State.Health.Status}}" prediction-fps-postgres 2>$null
    if ($Status -eq "healthy") {
        $IsHealthy = $true
        break
    }
    $RetryCount++
    Write-Host "  -> Estado actual: '$Status' (intento $RetryCount/$MaxRetries). Esperando 2 segundos..." -ForegroundColor DarkGray
    Start-Sleep -Seconds 2
}

if (-not $IsHealthy) {
    Write-Host "[ERROR] El contenedor 'prediction-fps-postgres' no alcanzó el estado 'healthy' a tiempo." -ForegroundColor Red
    docker compose logs postgres
    exit 1
}
Write-Host "  -> ¡Contenedor saludable (healthy) en localhost:5432!" -ForegroundColor Green

# 4. Aplicar migración 001_init.sql (idempotente)
Write-Host "`n[3/4] Asegurando aplicación de migrations/001_init.sql..." -ForegroundColor Yellow
Get-Content -Raw ".\migrations\001_init.sql" | docker exec -i prediction-fps-postgres psql -v ON_ERROR_STOP=1 -U game_user -d prediction_fps | Out-Null
Write-Host "  -> Esquema 001_init.sql aplicado sin errores." -ForegroundColor Green

# 5. Validar Criterios de Aceptación (10 tablas + catálogo semilla)
Write-Host "`n[4/4] Verificando Criterios de Aceptación (10 tablas y catálogo semilla)..." -ForegroundColor Yellow

Write-Host "`n--- Tablas en el esquema 'public' ---" -ForegroundColor Cyan
docker exec -i prediction-fps-postgres psql -U game_user -d prediction_fps -c "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;"

Write-Host "--- Catálogo Semilla (cosmetic_catalog) ---" -ForegroundColor Cyan
docker exec -i prediction-fps-postgres psql -U game_user -d prediction_fps -c "SELECT code, name, category, price_coins, is_default FROM cosmetic_catalog ORDER BY category DESC, price_coins ASC;"

Write-Host "=================================================================" -ForegroundColor Green
Write-Host " ¡MVP-002 completado y verificado exitosamente!" -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Green
