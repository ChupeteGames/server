package domain

import "context"

// HealthStatus representa el estado operativo del servicio dentro del dominio.
// Es un Value Object puro de Go sin etiquetas de transporte (JSON) ni dependencias externas.
type HealthStatus string

const (
	// StatusHealthy indica que el servidor y sus dependencias críticas (como PostgreSQL) están operativos.
	StatusHealthy HealthStatus = "ok"
	// StatusUnhealthy indica que alguna dependencia crítica falló la verificación de conectividad.
	StatusUnhealthy HealthStatus = "error"
)

// Health es la entidad/modelo de dominio que encapsula el estado de salud actual.
type Health struct {
	Status HealthStatus
}

// NewHealthyStatus construye un estado de dominio saludable ("ok").
func NewHealthyStatus() Health {
	return Health{Status: StatusHealthy}
}

// NewUnhealthyStatus construye un estado de dominio degradado/no saludable ("error").
func NewUnhealthyStatus() Health {
	return Health{Status: StatusUnhealthy}
}

// IsHealthy devuelve true si el servicio y sus dependencias se encuentran en estado óptimo.
func (h Health) IsHealthy() bool {
	return h.Status == StatusHealthy
}

// Pinger es el Puerto de Salida (Secondary / Driven Port) de la Arquitectura Hexagonal.
// Define el contrato que la Capa de Aplicación necesita para verificar si el
// almacenamiento subyacente (ej. PostgreSQL) está vivo, sin que el dominio ni la
// aplicación conozcan detalles del driver SQL (pgxpool).
type Pinger interface {
	Ping(ctx context.Context) error
}
