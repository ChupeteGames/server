package domain

import "errors"

// Errores tipados del dominio de Jugadores (players).
var (
	// ErrPlayerNotFound se devuelve cuando no existe un perfil para el player_id solicitado.
	ErrPlayerNotFound = errors.New("el perfil del jugador no fue encontrado")
)
