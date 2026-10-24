package domain

import (
	"strings"
	"time"
)

// GameMode representa el modo competitivo de la partida.
type GameMode string

const (
	Mode2v2 GameMode = "2v2"
	Mode3v3 GameMode = "3v3"
	Mode4v4 GameMode = "4v4"
	Mode5v5 GameMode = "5v5"
)

// MatchStatus representa las fases del ciclo de vida de una partida en PostgreSQL.
type MatchStatus string

const (
	MatchStatusCreated    MatchStatus = "created"
	MatchStatusInProgress MatchStatus = "in_progress"
	MatchStatusCompleted  MatchStatus = "completed"
	MatchStatusCancelled  MatchStatus = "cancelled"
)

// TeamSide representa el bando asignado al jugador dentro de la partida.
type TeamSide string

const (
	TeamSideA TeamSide = "TEAM_A"
	TeamSideB TeamSide = "TEAM_B"
)

// Match es la Entidad Raíz (Aggregate Root) que modela una sesión de partida competitiva.
type Match struct {
	ID              string
	Mode            GameMode
	Status          MatchStatus
	Outcome         string
	TotalRounds     int
	EndedInDuel     bool
	DurationSeconds int
	ServerHost      string
	ServerPort      int
	CreatedAt       time.Time
	StartedAt       *time.Time
	EndedAt         *time.Time
}

// ValidateMode normaliza y valida que el modo solicitado coincida con las modalidades del juego.
// Si el modo viene vacío, por convención del Vertical Slice (MVP) se asume "2v2".
func ValidateMode(mode string) (GameMode, error) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return Mode2v2, nil
	}
	switch GameMode(trimmed) {
	case Mode2v2, Mode3v3, Mode4v4, Mode5v5:
		return GameMode(trimmed), nil
	default:
		return "", ErrInvalidMode
	}
}

// RequiredPlayersPerTeam devuelve la cantidad de jugadores por bando según el modo.
func RequiredPlayersPerTeam(mode GameMode) int {
	switch mode {
	case Mode2v2:
		return 2
	case Mode3v3:
		return 3
	case Mode4v4:
		return 4
	case Mode5v5:
		return 5
	default:
		return 2
	}
}

// TotalPlayersRequired devuelve la cantidad total de jugadores requeridos para armar la partida.
func TotalPlayersRequired(mode GameMode) int {
	return RequiredPlayersPerTeam(mode) * 2
}
