# Elisanic — Game Backend Server (`server/`)

Servidor backend oficial del shooter competitivo de predicción en primera persona (**Chupete Games**). Construido en **Go** siguiendo estrictamente **Arquitectura Hexagonal (Ports & Adapters)**, **Domain-Driven Design (DDD)** y **Clean Architecture**, con **PostgreSQL 17** como base de datos transaccional.

---

## 1. Visión General y Canales de Comunicación

El backend actúa como la **única fuente de verdad** para cuentas, perfiles (MMR/XP), billeteras, tienda de cosméticos, matchmaking y cierre transaccional de partidas. Expone dos canales de entrada en simultáneo:

1. **API HTTP REST (`:8080` — Gin):** Consumida por el **Cliente de Unreal Engine** (jugadores) para chequeo de salud, login, perfil, tienda y cola de matchmaking.
2. **Servidor gRPC (`:9090` — Protobuf):** Consumido exclusivamente en red interna por el **Unreal Dedicated Server** para validar tickets de ingreso (`ValidateJoinTicket`), notificar inicio de partida (`MarkMatchStarted`) y reportar el acta final de la partida (`ReportMatchResult`).

```mermaid
flowchart LR
    subgraph Unreal["Ecosistema Unreal Engine"]
        Client["Cliente Unreal<br/>(PC del Jugador)"]
        DedSrv["Unreal Dedicated Server<br/>(Autoridad de Partida)"]
    end

    subgraph GoServer["Backend Go (server/) — Arquitectura Hexagonal"]
        direction TB
        subgraph InfraIn["Infraestructura: Adaptadores Primarios (Entrada)"]
            HTTP["Transporte HTTP (Gin)<br/>Puerto :8080"]
            GRPC["Transporte gRPC (Protobuf)<br/>Puerto :9090"]
        end

        subgraph App["Capa de Aplicación"]
            UC["Casos de Uso (UseCase)<br/>Orquestación + Trazabilidad (trace_id)"]
        end

        subgraph Dom["Capa de Dominio (100% Go Puro)"]
            Entities["Entidades, Value Objects,<br/>Puertos (Interfaces) y Errores"]
        end

        subgraph InfraOut["Infraestructura: Adaptadores Secundarios (Salida)"]
            PGAdapter["Repositorio PostgreSQL<br/>(pgxpool v5)"]
        end

        HTTP --> UC
        GRPC --> UC
        UC --> Entities
        PGAdapter -. "Implementa Puerto" .-> Entities
    end

    DB[("PostgreSQL 17<br/>localhost:5432<br/>(10 tablas + catálogo)")]

    Client -- "1. UDP Replicación" <--> DedSrv
    Client -- "2. HTTP/JSON + X-Trace-ID" --> HTTP
    DedSrv -- "3. gRPC + trace_id" --> GRPC
    PGAdapter -- "4. SQL Transaccional" --> DB
```

![Diagrama de Capas](./resources/diagrama-capas.png)

---

## 2. Inicio Rápido (Levantar todo en 2 minutos)

### Requisitos previos
- **Go 1.22+**
- **Docker Desktop** corriendo

### Paso 1: Levantar PostgreSQL 17 y aplicar migraciones
Desde la raíz de `server/`, ejecutá el script automatizado de base de datos:

```powershell
.\scripts\db-setup.ps1
```
> **¿Qué hace este script?**
> 1. Levanta el contenedor `prediction-fps-postgres` (`postgres:17`) en `localhost:5432` usando `docker-compose.yml`.
> 2. Espera hasta que el *healthcheck* reporte estado `healthy`.
> 3. Aplica de forma idempotente `migrations/001_init.sql`.
> 4. Lista en pantalla las **10 tablas creadas** y los **7 cosméticos del catálogo semilla**.

*(Alternativa manual sin script: `docker compose up -d`, ya que el contenedor monta `./migrations` en `/docker-entrypoint-initdb.d` y auto-aplica el SQL al crearse).*

### Paso 2: Levantar el servidor Go (HTTP + gRPC)
```powershell
go run ./cmd/api
```
El servidor ya trae valores por defecto pensados para el entorno local, por lo que no necesitás configurar variables de entorno para empezar. Verás en consola:
- `API HTTP escuchando en :8080`
- `Servidor gRPC escuchando en :9090`

---

## 3. Estructura del Proyecto y Arquitectura Hexagonal

Respetamos la **Regla de Dependencias**: las flechas siempre apuntan hacia adentro (`infrastructure` ➔ `application` ➔ `domain`).

```text
server/
├── cmd/
│   └── api/
│       └── main.go                  # Composition Root: carga Config, conecta pgxpool y levanta HTTP (:8080) + gRPC (:9090)
├── contracts/
│   └── proto/
│       └── game_backend.proto       # Contrato Protobuf oficial (Unreal Dedicated Server <-> Go)
├── internal/
│   ├── config/                      # Lectura y validación de variables de entorno (PORT, GRPC_PORT, DATABASE_URL, etc.)
│   ├── health/                      # Bounded Context: Chequeo de salud del servicio y la BD
│   │   ├── domain/                  # Go puro: Entidad Health, interfaz Pinger (Puerto) y errores tipados (errores.go)
│   │   ├── application/             # Caso de Uso: HealthUseCase (ejecuta Ping contra los puertos registrados)
│   │   └── infrastructure/
│   │       ├── storage/postgres/    # Adaptador Secundario: Pool pgxpool y repositorio que implementa domain.Pinger
│   │       └── transport/http/      # Adaptador Primario: Router Gin, middleware X-Trace-ID y HealthHandler
│   └── shared/
│       ├── pb/gamev1/               # Código Go generado (.pb.go) desde game_backend.proto (DTOs de transporte gRPC)
│       └── transport/grpc/          # Servidor gRPC (:9090), interceptor de trace_id, Health gRPC y Server Reflection
├── kit/
│   └── logging/                     # Interfaz Logger estructurado (slog en formato JSON)
├── migrations/
│   └── 001_init.sql                 # Esquema inicial idempotente (10 tablas + catálogo semilla de 7 cosméticos)
├── resources/                       # Diagramas visuales de arquitectura del equipo
├── scripts/
│   ├── db-setup.ps1                 # Levanta Postgres 17 y verifica criterios de aceptación de la BD
│   └── proto-gen.ps1                # Regenera los stubs Go (.pb.go) si se edita game_backend.proto
└── docker-compose.yml               # Orquestación local de PostgreSQL 17
```

### Reglas clave para agregar nuevos módulos (ej. `auth`, `players`, `matchmaking`, `economy`)
1. **`domain/` (100% Go puro):** Prohibido importar Gin, gRPC o `pgx`. Contiene entidades, lógica de negocio, puertos (interfaces de repositorios) y errores en `domain/errores.go`.
2. **`application/` (Casos de Uso):** Solo importa `domain/` y `kit/logging`. Recibe repositorios por inyección de dependencias en su constructor `New...UseCase(...)`. Todo método `Execute` recibe `(ctx context.Context, traceID string, ...)`.
3. **`infrastructure/` (Adaptadores):**
   - `storage/postgres/`: Consultas SQL con `pgxpool`.
   - `transport/http/` y `transport/grpc/`: Handlers y DTOs. Los DTOs nunca salen de la capa de transporte.

---

## 4. Variables de Entorno

Todas las variables tienen valores por defecto listos para usar en desarrollo local:

| Variable | Valor por defecto (Local) | Descripción |
| :--- | :--- | :--- |
| `PORT` | `8080` | Puerto del servidor HTTP REST (Gin) para Clientes Unreal. |
| `GRPC_PORT` | `9090` | Puerto del servidor gRPC para el Unreal Dedicated Server. |
| `GIN_MODE` | `release` | Modo de Gin (`debug`, `release` o `test`). |
| `DATABASE_URL` | `postgres://game_user:local_dev_only_change_me@localhost:5432/prediction_fps?sslmode=disable` | Cadena de conexión a PostgreSQL 17. |
| `READ_HEADER_TIMEOUT` | `5s` | Tiempo máximo para leer headers HTTP. |
| `SHUTDOWN_TIMEOUT` | `5s` | Tiempo máximo de espera para *graceful shutdown* (HTTP y gRPC). |

---

## 5. Cómo Probar los Servicios Localmente

### A. Probar la API HTTP (`:8080`)
Con el servidor corriendo, ejecutá desde PowerShell o abrí en el navegador:

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/health" -Method Get -Headers @{"X-Trace-ID"="mi-traza-123"}
```
- **Respuesta (`200 OK`):** `{"status":"ok"}` (verifica en tiempo real mediante `pgxpool.Ping` que PostgreSQL esté respondiendo).
- **Si PostgreSQL está apagado (`503 Service Unavailable`):** `{"status":"error"}`.

### B. Probar el Servidor gRPC (`:9090`) con Postman / Insomnia
El servidor gRPC tiene **Server Reflection** habilitado, por lo que no necesitás cargar el archivo `.proto` manualmente:
1. Abrí **Postman** ➔ **New** ➔ **gRPC**.
2. Ingresá la URL `localhost:9090`.
3. Hacé clic en **Select a method** y elegí cualquiera de los métodos descubiertos automáticamente:
   - `grpc.health.v1.Health / Check` ➔ responde `"status": "SERVING"`.
   - `game.v1.GameBackendService / ValidateJoinTicket`
   - `game.v1.GameBackendService / MarkMatchStarted`
   - `game.v1.GameBackendService / ReportMatchResult`
4. Todos los métodos leen `trace_id` (desde el mensaje Protobuf o el metadata `x-trace-id`), lo registran en el log JSON estructurado de Go y lo devuelven en los headers de respuesta gRPC.

---

## 6. Mapa de la Base de Datos PostgreSQL 17 (`migrations/001_init.sql`)

| # | Tabla | Bounded Context | Propósito Principal |
| :--- | :--- | :--- | :--- |
| 1 | `users` | `auth` | Credenciales de cuenta (`username`, `password_hash` con bcrypt). |
| 2 | `cosmetic_catalog` | `economy` | Catálogo de tienda (7 ítems semilla: 4 `skin` estilo Fall Guys y 3 `death_effect`). |
| 3 | `player_profiles` | `players` | Nivel, XP, ranking (`mmr` base 1000), estadísticas de carrera y *loadout* equipado. |
| 4 | `wallets` | `economy` | Saldo de monedas del jugador con invariante `CHECK (balance >= 0)`. |
| 5 | `player_cosmetics` | `economy` | Inventario de cosméticos desbloqueados por cada jugador. |
| 6 | `economy_transactions` | `economy` | Libro contable inmutable con `operation_id` (UUID) para garantizar **idempotencia**. |
| 7 | `matches` | `matchmaking` / `matches` | Ciclo de vida de partidas (`2v2` a `5v5`), estado, resultado y `report_operation_id` único. |
| 8 | `match_tickets` | `matchmaking` | Tickets temporales de ingreso al Unreal Dedicated Server y equipo asignado (`TEAM_A`/`TEAM_B`). |
| 9 | `match_rounds` | `matches` | Historial por ronda (bajas, consumo/muertes por *Ronda de Gracia*, timer 3m, Duelo 1v1). |
| 10 | `match_player_stats` | `matches` | Estadísticas individuales por partida y recompensas otorgadas (`mmr_delta`, monedas, XP). |

### Cómo explorar las tablas visualmente (GoLand / DataGrip / DBeaver)
- **Host:** `localhost` | **Port:** `5432`
- **Database:** `prediction_fps`
- **User:** `game_user` | **Password:** `local_dev_only_change_me`

---

## 7. Calidad de Código, Tests y Flujo de Git

Antes de subir cualquier cambio o abrir un Pull Request, ejecutá siempre los 3 comandos de verificación en `server/`:

```powershell
gofmt -s -w .
go vet ./...
go test -v ./...
```

### Convenciones de Git del Equipo
- **Ramas:** `feature/xxxx-xxx-xx` (en minúsculas y separadas por guiones, ej. `feature/grpc-server-proto`).
- **Commits:** Una sola oración breve y clara que resuma el cambio realizado.
