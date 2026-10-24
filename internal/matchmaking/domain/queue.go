package domain

import (
	"context"
	"time"
)

// QueueStatus representa el estado actual de un jugador en el proceso de matchmaking.
type QueueStatus string

const (
	StatusIdle       QueueStatus = "IDLE"
	StatusQueued     QueueStatus = "QUEUED"
	StatusMatchFound QueueStatus = "MATCH_FOUND"
)

// PlayerQueueEntry representa la ficha de un jugador esperando ser emparejado en memoria.
type PlayerQueueEntry struct {
	PlayerID   string
	Mode       GameMode
	EnqueuedAt time.Time
}

// MatchmakingResult contiene los datos necesarios para que el cliente Unreal se conecte al Dedicated Server
// una vez que la partida fue conformada por el sistema.
type MatchmakingResult struct {
	Status       QueueStatus
	Mode         GameMode
	MatchID      string
	ServerHost   string
	ServerPort   int
	JoinTicket   string
	AssignedTeam TeamSide
	ExpiresAt    *time.Time
}

// MatchRepository es el Puerto de Salida (Secondary Port) para la persistencia transaccional
// de partidas y tickets de acceso en PostgreSQL.
type MatchRepository interface {
	// CreateMatchWithTickets inserta en una única transacción ACID el registro de la partida (matches)
	// y los tickets de ingreso asociados a cada jugador (match_tickets).
	CreateMatchWithTickets(ctx context.Context, match Match, tickets []MatchTicket) error

	// GetMatchByID recupera una partida por su identificador UUID.
	GetMatchByID(ctx context.Context, matchID string) (Match, error)

	// FindActiveTicketByPlayerID busca si el jugador tiene un ticket emitido recientemente sin consumir
	// para una partida activa (created o in_progress).
	FindActiveTicketByPlayerID(ctx context.Context, playerID string) (*MatchTicket, *Match, error)

	// ValidateAndConsumeTicket verifica la validez del ticket (coincidencia de match_id, expiración y uso previo),
	// lo marca como consumido atómicamente y obtiene el perfil competitivo y cosméticos del jugador.
	ValidateAndConsumeTicket(ctx context.Context, matchID, ticketToken string) (ValidatedTicket, error)

	// MarkMatchStarted actualiza el estado de la partida a 'in_progress' y registra el momento de inicio.
	MarkMatchStarted(ctx context.Context, matchID string) error
}

// MatchmakingQueue es el Puerto de Salida para la administración concurrente en memoria de la cola de espera.
type MatchmakingQueue interface {
	// Enqueue añade al jugador a la cola. Si se alcanza el cupo total requerido para el modo (ej. 4 para 2v2),
	// devuelve isMatchFormed=true junto con el lote de jugadores emparejados.
	Enqueue(ctx context.Context, entry PlayerQueueEntry) (isMatchFormed bool, matchedPlayers []PlayerQueueEntry, err error)

	// Dequeue retira al jugador de la cola de espera.
	Dequeue(ctx context.Context, playerID string) error

	// GetStatus consulta el estado del jugador en memoria (en cola o con partida lista).
	GetStatus(ctx context.Context, playerID string) (QueueStatus, *PlayerQueueEntry, *MatchmakingResult, error)

	// StoreMatchResult almacena temporalmente el resultado del emparejamiento para que el jugador
	// pueda consultarlo vía polling al consultar /v1/matchmaking/status.
	StoreMatchResult(ctx context.Context, playerID string, result MatchmakingResult)
}
