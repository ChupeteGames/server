<#
.SYNOPSIS
    Script para regenerar el código Go (.pb.go y _grpc.pb.go) a partir de
    contracts/proto/game_backend.proto cuando se modifique el contrato.

.DESCRIPTION
    Requiere tener instalado 'protoc' en el sistema. Instala automáticamente
    las versiones compatibles de protoc-gen-go y protoc-gen-go-grpc si no existen.
#>

$ErrorActionPreference = "Stop"

$ServerRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ServerRoot

if (-not (Get-Command "protoc" -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] 'protoc' no está instalado en el PATH." -ForegroundColor Red
    Write-Host "Los archivos ya están pre-generados en internal/shared/pb/gamev1/." -ForegroundColor Yellow
    Write-Host "Si editaste contracts/proto/game_backend.proto, instala protoc (ej. winget install Google.Protobuf) y vuelve a correr este script." -ForegroundColor Yellow
    exit 1
}

Write-Host "Generando stubs Go desde contracts/proto/game_backend.proto..." -ForegroundColor Cyan
protoc `
    --proto_path=contracts/proto `
    --go_out=internal/shared/pb/gamev1 --go_opt=paths=source_relative `
    --go-grpc_out=internal/shared/pb/gamev1 --go-grpc_opt=paths=source_relative `
    contracts/proto/game_backend.proto

Write-Host "¡Archivos generados exitosamente en internal/shared/pb/gamev1/!" -ForegroundColor Green
