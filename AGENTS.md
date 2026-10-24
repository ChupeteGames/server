# Reglas de Desarrollo y Arquitectura — Proyecto Elisanic (`server/`)

## 1. Estilo de Trabajo y Comunicación
- **Desarrollo paso a paso y didáctico:** Al implementar cualquier feature o cambio en `server/`, explica de forma clara qué se está haciendo y por qué antes o durante el avance, para que el usuario comprenda el flujo completo.
- **Código autodocumentado y comentado:** Incluye comentarios explicativos en español dentro del código (estructuras, puertos, casos de uso, adaptadores y scripts SQL) detallando la lógica de negocio y qué rol cumple cada pieza dentro de la arquitectura.
- **Limpieza y documentación viva:** No subas binarios temporales de prueba manual (ej. en `cmd/`) al repositorio; todas las pruebas deben quedar como tests automatizados (`*_test.go`) y manteniendo `README.md` actualizado cuando se sumen nuevos módulos, rutas o variables de entorno.

## 2. Arquitectura Hexagonal, DDD y Clean Architecture (Go)
Respeta estrictamente la estructura de directorios de `server/` y las reglas de `.github/copilot-instructions.md`:
- **Regla de Dependencias:** Las dependencias siempre apuntan hacia adentro (`infrastructure` -> `application` -> `domain`).
- **Capa de Dominio (`internal/[contexto]/domain/`):**
  - Debe ser **Go 100% puro** (sin importar Gin, gRPC, `pgx` ni librerías externas).
  - Contiene Entidades, Agregados, Value Objects, Interfaces de Repositorios/Servicios (Puertos) y errores tipados en `domain/errores.go`.
- **Capa de Aplicación (`internal/[contexto]/application/`):**
  - Contiene los Casos de Uso (`UseCase`). Solo importa `domain/` y `kit/logging`.
  - Recibe puertos por Inyección de Dependencias en su constructor (`New...UseCase`).
  - Los métodos `Execute` reciben `ctx context.Context` y `traceID string`.
- **Capa de Infraestructura (`internal/[contexto]/infrastructure/`):**
  - `storage/postgres/`: Implementaciones concretas de los repositorios usando `pgxpool`.
  - `transport/http/` y `transport/grpc/`: Handlers, middlewares (`X-Trace-ID`) y DTOs. Los DTOs nunca deben mezclarse con las entidades de `domain/`.
- **Verificación continua:** Antes de dar por cerrada una tarea en Go, ejecuta `gofmt -s -w .`, `go vet ./...` y `go test -v ./...`.

## 3. Contexto y Documentación del Dominio
- Cuando necesites contexto sobre reglas del juego, contratos o alcance del MVP, consulta los documentos en `prediction_fps_paquete_inicial/docs/` (`01` a `07`), los contratos en `contracts/` (`openapi.yaml` y `game_backend.proto`) y las tareas en `prediction_fps_paquete_inicial/backlog/backlog.csv`.

## 4. Convenciones de Git (Branches y Commits)
- **Flujo de ramas:** Antes de comenzar una nueva tarea, párate siempre sobre `main` actualizado (`git checkout main && git pull origin main`) y crea una rama nueva.
- **Formato de ramas:** Usa siempre el prefijo y formato `feature/xxxx-xxx-xx`, reemplazando las `x` con un título breve descriptivo en minúsculas separado por guiones (ej. `feature/postgres-init-connection`).
- **Formato de commits:** Escribe mensajes de commit claros y breves de **una sola oración** que resuman el cambio realizado.
