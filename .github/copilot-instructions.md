# Golang Architecture and Folder Structure Instructions

You are an expert Go developer enforcing strict Hexagonal Architecture, Domain-Driven Design (DDD), and Clean Architecture principles. When writing or refactoring code, you must strictly adhere to the following directory layout and dependency rules.

## 1. Directory Tree Structure
Every microservice/bounded context must follow this exact hierarchy:
- `cmd/api/main.go`: Application entry point. Initializes dependencies and starts the transport server.
- `internal/[context]/domain/`: Domain Layer. Pure business logic, Aggregates, Entities, Value Objects, Domain Errors, and Repository/Service Interfaces (Ports).
- `internal/[context]/application/`: Application Layer. Use Cases orchestrating the domain logic. Contains the workflows and interacts only with domain interfaces.
- `internal/[context]/infrastructure/storage/[driver]/`: Output Adapters (Secondary). Database, Cache, or External API implementations (e.g., MySQL, MongoDB, Redis) matching domain repository interfaces.
- `internal/[context]/infrastructure/transport/[protocol]/`: Input Adapters (Primary). Handlers and DTOs for HTTP (Gin/Fiber/Chi), gRPC, or Event Consumers (Kafka/RabbitMQ).
- `kit/`: (Optional) Shared internal utilities (e.g., logger, custom types).

## 2. Dependency Rules (Crucial)
- Dependencies must only point inwards: Infrastructure -> Application -> Domain.
- **The Domain Layer (`domain/`) must be pure Go.** It MUST NOT import anything from `application/`, `infrastructure/`, or any external third-party frameworks (like Gorm, Gin, etc.).
- **The Application Layer (`application/`)** can only import `domain/`. It must never import `infrastructure/`. It must use Dependency Injection via interfaces defined in the domain.
- **The Infrastructure Layer (`infrastructure/`)** imports both `domain/` and `application/` to implement ports and drive the application.

## 3. Code Generation Requirements
- Always organize new files into this folder structure based on their architectural responsibility.
- Implement explicit Dependency Injection (typically through constructors like `NewUseCase` or `NewRepository`).
- Define repository interfaces inside `domain/` and their concrete implementations inside `infrastructure/storage/`.
- Ensure domain errors are typed errors defined within `domain/errores.go`.
- Keep transport models (DTOs) strictly within the `transport/` folder; do not mix them with domain entities.
