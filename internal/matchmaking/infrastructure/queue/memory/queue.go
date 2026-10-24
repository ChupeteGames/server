package memory

import (
	"context"
	"sync"
	"time"

	"server/internal/matchmaking/domain"
)

// MemoryQueue es la implementación concurrente en memoria de domain.MatchmakingQueue.
// Mantiene las listas de espera de jugadores agrupadas por modo de juego (FIFO)
// y preserva los resultados de partidas asignadas para consultas subsecuentes.
type MemoryQueue struct {
	mu           sync.RWMutex
	queues       map[domain.GameMode][]domain.PlayerQueueEntry
	playerMode   map[string]domain.GameMode
	matchResults map[string]domain.MatchmakingResult
}

var _ domain.MatchmakingQueue = (*MemoryQueue)(nil)

// NewMemoryQueue construye una instancia de la cola en memoria.
func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		queues:       make(map[domain.GameMode][]domain.PlayerQueueEntry),
		playerMode:   make(map[string]domain.GameMode),
		matchResults: make(map[string]domain.MatchmakingResult),
	}
}

// Enqueue añade al jugador al final de la cola del modo correspondiente.
// Si se alcanza el cupo total (ej. 4 jugadores para 2v2), extrae el lote y retorna isMatchFormed=true.
func (q *MemoryQueue) Enqueue(
	_ context.Context,
	entry domain.PlayerQueueEntry,
) (bool, []domain.PlayerQueueEntry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Si el jugador ya figura en espera en algún modo, rechazamos la duplicación
	if _, exists := q.playerMode[entry.PlayerID]; exists {
		return false, nil, domain.ErrAlreadyInQueue
	}

	q.queues[entry.Mode] = append(q.queues[entry.Mode], entry)
	q.playerMode[entry.PlayerID] = entry.Mode

	required := domain.TotalPlayersRequired(entry.Mode)
	if len(q.queues[entry.Mode]) >= required {
		matched := make([]domain.PlayerQueueEntry, required)
		copy(matched, q.queues[entry.Mode][:required])
		q.queues[entry.Mode] = q.queues[entry.Mode][required:]

		for _, p := range matched {
			delete(q.playerMode, p.PlayerID)
		}

		return true, matched, nil
	}

	return false, nil, nil
}

// Dequeue remueve al jugador de la lista de espera del modo en que se encuentre.
func (q *MemoryQueue) Dequeue(_ context.Context, playerID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	mode, ok := q.playerMode[playerID]
	if !ok {
		return domain.ErrNotInQueue
	}

	entries := q.queues[mode]
	filtered := make([]domain.PlayerQueueEntry, 0, len(entries))
	for _, e := range entries {
		if e.PlayerID != playerID {
			filtered = append(filtered, e)
		}
	}
	q.queues[mode] = filtered
	delete(q.playerMode, playerID)
	return nil
}

// GetStatus recupera el estado del jugador: si ya tiene una partida asignada o si sigue en cola.
func (q *MemoryQueue) GetStatus(
	_ context.Context,
	playerID string,
) (domain.QueueStatus, *domain.PlayerQueueEntry, *domain.MatchmakingResult, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// 1. Verificamos si tiene una partida ya asignada
	if result, ok := q.matchResults[playerID]; ok {
		// Si el ticket ya expiró, lo ignoramos
		if result.ExpiresAt == nil || time.Now().UTC().Before(*result.ExpiresAt) {
			resCopy := result
			return domain.StatusMatchFound, nil, &resCopy, nil
		}
	}

	// 2. Verificamos si sigue en cola de espera
	if mode, ok := q.playerMode[playerID]; ok {
		entry := &domain.PlayerQueueEntry{
			PlayerID: playerID,
			Mode:     mode,
		}
		return domain.StatusQueued, entry, nil, nil
	}

	return domain.StatusIdle, nil, nil, nil
}

// StoreMatchResult almacena el resultado del emparejamiento para que el jugador pueda consumirlo.
func (q *MemoryQueue) StoreMatchResult(
	_ context.Context,
	playerID string,
	result domain.MatchmakingResult,
) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.matchResults[playerID] = result
}
