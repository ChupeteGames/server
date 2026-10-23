package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"server/internal/health/domain"
)

const (
	// defaultPingTimeout acota el tiempo máximo que esperamos al verificar la conexión inicial.
	defaultPingTimeout = 5 * time.Second
)

// Repository es el Adaptador Secundario (Output Adapter) para PostgreSQL dentro del
// contexto de Health. Implementa la interfaz domain.Pinger utilizando un pool de
// conexiones concurrentes de pgx (pgxpool.Pool).
type Repository struct {
	pool *pgxpool.Pool
}

// Verificación en tiempo de compilación: garantiza que *Repository cumple el puerto domain.Pinger.
var _ domain.Pinger = (*Repository)(nil)

// NewPool inicializa un pool de conexiones a PostgreSQL (pgxpool.Pool) a partir de la
// URL de conexión (DATABASE_URL) y ejecuta inmediatamente un Ping para validar que la
// base de datos en localhost:5432 esté accesible y saludable antes de levantar la API.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configuración de DATABASE_URL inválida: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el pool de conexiones a PostgreSQL: %w", err)
	}

	// Verificamos que la conexión real hacia PostgreSQL 17 funcione al iniciar el servidor.
	pingCtx, cancel := context.WithTimeout(ctx, defaultPingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("no se pudo conectar a PostgreSQL (ping inicial falló): %w", err)
	}

	return pool, nil
}

// NewRepository construye el adaptador de persistencia de salud inyectándole el pool de PostgreSQL.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Ping verifica que el pool de conexiones de PostgreSQL siga respondiendo consultas.
// Satisface el puerto domain.Pinger requerido por la Capa de Aplicación.
func (r *Repository) Ping(ctx context.Context) error {
	if r.pool == nil {
		return domain.ErrStorageUnavailable
	}
	return r.pool.Ping(ctx)
}
