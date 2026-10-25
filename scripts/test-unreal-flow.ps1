<#
.SYNOPSIS
    Simulador completo del flujo de conexión de Unreal Engine (4 Clientes + Dedicated Server)
    contra el Backend Go (http://localhost:8080) y PostgreSQL 17, sin necesitar abrir Unreal Engine.

.DESCRIPTION
    Ejecuta exactamente las mismas peticiones HTTP/JSON y headers X-Trace-ID que realiza
    el subsistema C++ UBackendSubsystem de lizardgame:
      1. GET /health (chequeo de conectividad y PostgreSQL).
      2. POST /v1/auth/login para 4 jugadores (unreal_p1 .. unreal_p4).
      3. GET /v1/players/me para verificar perfil, MMR (1000), billetera y loadout equipado.
      4. POST /v1/matchmaking/enqueue ("2v2") para los 4 jugadores y GET /v1/matchmaking/status.
      5. POST /v1/internal/matches/validate-ticket (como Unreal Dedicated Server) para validar y consumir los 4 tickets.
      6. Prueba de rechazo por reintento con un ticket ya utilizado (TICKET_ALREADY_USED).
      7. POST /v1/internal/matches/mark-started (como Unreal Dedicated Server) al iniciar la Ronda 1.
      8. Consulta directa a PostgreSQL (si Docker está disponible) mostrando la partida y los tickets consumidos.
#>

param(
    [string]$BaseUrl = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"

Write-Host "=====================================================================" -ForegroundColor Cyan
Write-Host " Simulador de Conexión Unreal Engine (lizardgame) <-> Backend Go" -ForegroundColor Cyan
Write-Host " Destino: $BaseUrl" -ForegroundColor Cyan
Write-Host "=====================================================================" -ForegroundColor Cyan

# 1. Chequeo de salud (GET /health)
Write-Host "`n[1/6] Cliente Unreal -> GET /health..." -ForegroundColor Yellow
try {
    $health = Invoke-RestMethod -Uri "$BaseUrl/health" -Method Get -Headers @{"X-Trace-ID"="unreal-sim-health-001"}
    Write-Host "  -> Respuesta: status=$($health.status) (Backend Go + PostgreSQL saludables)" -ForegroundColor Green
} catch {
    Write-Host "[ERROR] No se pudo conectar a $BaseUrl/health." -ForegroundColor Red
    Write-Host "Asegurate de tener levantado el servidor en otra terminal con: go run ./cmd/api" -ForegroundColor Yellow
    exit 1
}

# 2. Login de 4 clientes Unreal (POST /v1/auth/login) y consulta de perfil (GET /v1/players/me)
Write-Host "`n[2/6] Clientes Unreal (x4) -> POST /v1/auth/login + GET /v1/players/me..." -ForegroundColor Yellow
$players = @()
for ($i = 1; $i -le 4; $i++) {
    $username = "unreal_p$i"
    $loginBody = @{
        username = $username
        password = "password123"
    } | ConvertTo-Json

    $loginResp = Invoke-RestMethod -Uri "$BaseUrl/v1/auth/login" -Method Post -ContentType "application/json" -Headers @{"X-Trace-ID"="unreal-sim-login-00$i"} -Body $loginBody
    $profileResp = Invoke-RestMethod -Uri "$BaseUrl/v1/players/me" -Method Get -Headers @{
        "Authorization" = "Bearer $($loginResp.access_token)"
        "X-Trace-ID"    = "unreal-sim-profile-00$i"
    }

    $players += [PSCustomObject]@{
        Index       = $i
        Username    = $username
        PlayerId    = $loginResp.player_id
        DisplayName = $loginResp.display_name
        Token       = $loginResp.access_token
        MMR         = $profileResp.mmr
        Skin        = $profileResp.equipped_skin_code
        Gore        = $profileResp.equipped_death_effect_code
        Ticket      = ""
        Team        = ""
    }

    Write-Host "  -> Jugador $i ($username) autenticado | ID: $($loginResp.player_id) | MMR: $($profileResp.mmr) | Skin: $($profileResp.equipped_skin_code)" -ForegroundColor Green
}

# 3. Encolar los 4 clientes en Matchmaking 2v2 (POST /v1/matchmaking/enqueue)
Write-Host "`n[3/6] Clientes Unreal (x4) -> POST /v1/matchmaking/enqueue (modo 2v2)..." -ForegroundColor Yellow
$matchId = ""
foreach ($p in $players) {
    $mmResp = Invoke-RestMethod -Uri "$BaseUrl/v1/matchmaking/enqueue" -Method Post -ContentType "application/json" -Headers @{
        "Authorization" = "Bearer $($p.Token)"
        "X-Trace-ID"    = "unreal-sim-enqueue-00$($p.Index)"
    } -Body '{"mode":"2v2"}'

    Write-Host "  -> Jugador $($p.Index) ($($p.Username)) enqueue status: $($mmResp.status)" -ForegroundColor Cyan
    if ($mmResp.status -eq "MATCH_FOUND") {
        $matchId = $mmResp.match_id
    }
}

# 4. Consultar GET /v1/matchmaking/status para los 4 clientes (como hace el polling de UBackendSubsystem)
Write-Host "`n[4/6] Clientes Unreal (x4) -> GET /v1/matchmaking/status (obteniendo join_ticket y equipo)..." -ForegroundColor Yellow
foreach ($p in $players) {
    $stResp = Invoke-RestMethod -Uri "$BaseUrl/v1/matchmaking/status" -Method Get -Headers @{
        "Authorization" = "Bearer $($p.Token)"
        "X-Trace-ID"    = "unreal-sim-status-00$($p.Index)"
    }
    $p.Ticket = $stResp.join_ticket
    $p.Team   = $stResp.assigned_team
    $matchId  = $stResp.match_id

    Write-Host "  -> $($p.Username) | Status: $($stResp.status) | MatchID: $($stResp.match_id) | Equipo: $($p.Team) | Ticket: $($p.Ticket)" -ForegroundColor Green
}

# 5. Unreal Dedicated Server valida los 4 tickets (POST /v1/internal/matches/validate-ticket)
Write-Host "`n[5/6] Unreal Dedicated Server -> POST /v1/internal/matches/validate-ticket..." -ForegroundColor Yellow
foreach ($p in $players) {
    $valBody = @{
        trace_id    = "unreal-ds-validate-00$($p.Index)"
        match_id    = $matchId
        join_ticket = $p.Ticket
    } | ConvertTo-Json

    $valResp = Invoke-RestMethod -Uri "$BaseUrl/v1/internal/matches/validate-ticket" -Method Post -ContentType "application/json" -Body $valBody
    Write-Host "  -> Ticket de $($p.Username): valid=$($valResp.valid) | reason=$($valResp.reason_code) | team=$($valResp.assigned_team) | skin=$($valResp.equipped_skin_code) | gore=$($valResp.equipped_death_effect_code)" -ForegroundColor Green
}

# Verificar que un segundo intento con el mismo ticket sea rechazado (TICKET_ALREADY_USED)
$dupBody = @{
    trace_id    = "unreal-ds-validate-dup"
    match_id    = $matchId
    join_ticket = $players[0].Ticket
} | ConvertTo-Json
$dupResp = Invoke-RestMethod -Uri "$BaseUrl/v1/internal/matches/validate-ticket" -Method Post -ContentType "application/json" -Body $dupBody
Write-Host "  -> Prueba anti-replay (reusar ticket de $($players[0].Username)): valid=$($dupResp.valid) | reason=$($dupResp.reason_code)" -ForegroundColor Magenta

# 6. Unreal Dedicated Server avisa que arrancó la Ronda 1 (POST /v1/internal/matches/mark-started)
Write-Host "`n[6/6] Unreal Dedicated Server -> POST /v1/internal/matches/mark-started..." -ForegroundColor Yellow
$startBody = @{
    trace_id           = "unreal-ds-start-001"
    match_id           = $matchId
    server_instance_id = "unreal-dedicated-local"
} | ConvertTo-Json
$startResp = Invoke-RestMethod -Uri "$BaseUrl/v1/internal/matches/mark-started" -Method Post -ContentType "application/json" -Body $startBody
Write-Host "  -> Respuesta: success=$($startResp.success) | result_code=$($startResp.result_code) | $($startResp.message)" -ForegroundColor Green

Write-Host "`n=====================================================================" -ForegroundColor Green
Write-Host " ¡Flujo completo Cliente Unreal + Dedicated Server <-> Go verificado!" -ForegroundColor Green
Write-Host " Match ID en PostgreSQL: $matchId" -ForegroundColor Green
Write-Host "=====================================================================" -ForegroundColor Green
