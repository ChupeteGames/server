package domain

import "errors"

// Errores de dominio tipados para el contexto de salud (Health).
// Siguiendo las reglas de DDD (.github/copilot-instructions.md), los errores
// propios del negocio/dominio viven en domain/errores.go usando solo la librería estándar.
var (
	// ErrStorageUnavailable indica que la base de datos o el adaptador de persistencia no respondió al ping.
	ErrStorageUnavailable = errors.New("el almacenamiento de datos no se encuentra disponible")
)
