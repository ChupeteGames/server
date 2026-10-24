package application_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"server/internal/matchmaking/application"
	"server/internal/matchmaking/domain"
)

type mockMatchRepo struct {
	mu           sync.Mutex
	matches      map[string]domain.Match
	tickets      map[string]domain.MatchTicket
	validTickets map[string]domain.ValidatedTicket
}

func newMockMatchRepo() *mockMatchRepo {
	return &mockMatchRepo{
		matches:      make(map[string]domain.Match),
		tickets:      make(map[string]domain.MatchTicket),
		validTickets: make(map[string]domain.ValidatedTicket),
	}
}

func (m *mockMatchRepo) CreateMatchWithTickets(_ context.Context, match domain.Match, tickets []domain.MatchTicket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.matches[match.ID] = match
	for _, t := range tickets {
		m.tickets[t.TicketToken] = t
	}
	return nil
}

func (m *mockMatchRepo) GetMatchByID(_ context.Context, matchID string) (domain.Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	match, ok := m.matches[matchID]
	if !ok {
		return domain.Match{}, domain.ErrMatchNotFound
	}
	return match, nil
}

func (m *mockMatchRepo) FindActiveTicketByPlayerID(_ context.Context, playerID string) (*domain.MatchTicket, *domain.Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tickets {
		if t.PlayerID == playerID && t.ConsumedAt == nil && time.Now().UTC().Before(t.ExpiresAt) {
			match := m.matches[t.MatchID]
			ticketCopy := t
			return &ticketCopy, &match, nil
		}
	}
	return nil, nil, nil
}

func (m *mockMatchRepo) ValidateAndConsumeTicket(_ context.Context, matchID, ticketToken string) (domain.ValidatedTicket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket, ok := m.tickets[ticketToken]
	if !ok {
		return domain.ValidatedTicket{Valid: false, ReasonCode: "TICKET_NOT_FOUND"}, nil
	}
	if ticket.MatchID != matchID {
		return domain.ValidatedTicket{Valid: false, ReasonCode: "MATCH_MISMATCH"}, nil
	}
	if ticket.ConsumedAt != nil {
		return domain.ValidatedTicket{Valid: false, ReasonCode: "TICKET_ALREADY_USED"}, nil
	}
	if time.Now().UTC().After(ticket.ExpiresAt) {
		return domain.ValidatedTicket{Valid: false, ReasonCode: "TICKET_EXPIRED"}, nil
	}

	now := time.Now().UTC()
	ticket.ConsumedAt = &now
	m.tickets[ticketToken] = ticket

	res, ok := m.validTickets[ticketToken]
	if !ok {
		res = domain.ValidatedTicket{
			Valid:                   true,
			ReasonCode:              "OK",
			PlayerID:                ticket.PlayerID,
			DisplayName:             "JugadorPrueba",
			AssignedTeam:            ticket.AssignedTeam,
			EquippedSkinCode:        "bean_knight",
			EquippedDeathEffectCode: "gore_classic",
			CurrentMMR:              1050,
		}
	}
	return res, nil
}

func (m *mockMatchRepo) MarkMatchStarted(_ context.Context, matchID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	match, ok := m.matches[matchID]
	if !ok {
		return domain.ErrMatchNotFound
	}
	match.Status = domain.MatchStatusInProgress
	now := time.Now().UTC()
	match.StartedAt = &now
	m.matches[matchID] = match
	return nil
}

type mockQueue struct {
	mu            sync.Mutex
	queue         map[domain.GameMode][]domain.PlayerQueueEntry
	results       map[string]domain.MatchmakingResult
	playerModeMap map[string]domain.GameMode
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		queue:         make(map[domain.GameMode][]domain.PlayerQueueEntry),
		results:       make(map[string]domain.MatchmakingResult),
		playerModeMap: make(map[string]domain.GameMode),
	}
}

func (q *mockQueue) Enqueue(_ context.Context, entry domain.PlayerQueueEntry) (bool, []domain.PlayerQueueEntry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.playerModeMap[entry.PlayerID]; exists {
		return false, nil, domain.ErrAlreadyInQueue
	}

	q.queue[entry.Mode] = append(q.queue[entry.Mode], entry)
	q.playerModeMap[entry.PlayerID] = entry.Mode

	needed := domain.TotalPlayersRequired(entry.Mode)
	if len(q.queue[entry.Mode]) >= needed {
		matched := q.queue[entry.Mode][:needed]
		q.queue[entry.Mode] = q.queue[entry.Mode][needed:]
		for _, p := range matched {
			delete(q.playerModeMap, p.PlayerID)
		}
		return true, matched, nil
	}

	return false, nil, nil
}

func (q *mockQueue) Dequeue(_ context.Context, playerID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	mode, ok := q.playerModeMap[playerID]
	if !ok {
		return domain.ErrNotInQueue
	}

	entries := q.queue[mode]
	filtered := make([]domain.PlayerQueueEntry, 0, len(entries))
	for _, e := range entries {
		if e.PlayerID != playerID {
			filtered = append(filtered, e)
		}
	}
	q.queue[mode] = filtered
	delete(q.playerModeMap, playerID)
	return nil
}

func (q *mockQueue) GetStatus(_ context.Context, playerID string) (domain.QueueStatus, *domain.PlayerQueueEntry, *domain.MatchmakingResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if res, ok := q.results[playerID]; ok {
		return domain.StatusMatchFound, nil, &res, nil
	}

	if mode, ok := q.playerModeMap[playerID]; ok {
		return domain.StatusQueued, &domain.PlayerQueueEntry{
			PlayerID: playerID,
			Mode:     mode,
		}, nil, nil
	}

	return domain.StatusIdle, nil, nil, nil
}

func (q *mockQueue) StoreMatchResult(_ context.Context, playerID string, result domain.MatchmakingResult) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.results[playerID] = result
}

func TestMatchmakingFourPlayersEnqueueFlow(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockMatchRepo()
	queue := newMockQueue()

	enqueueUC := application.NewEnqueueUseCase(logger, repo, queue, "127.0.0.1", 7777)
	statusUC := application.NewGetStatusUseCase(logger, repo, queue)

	p1 := "user-111"
	p2 := "user-222"
	p3 := "user-333"
	p4 := "user-444"

	// Jugador 1 se encola
	res1, err := enqueueUC.Execute(ctx, "trace-1", p1, "2v2")
	if err != nil {
		t.Fatalf("error encolando jugador 1: %v", err)
	}
	if res1.Status != domain.StatusQueued {
		t.Fatalf("se esperaba estado QUEUED para jugador 1, se obtuvo: %v", res1.Status)
	}

	// Consultar estado de P1
	status1, err := statusUC.Execute(ctx, "trace-1", p1)
	if err != nil {
		t.Fatalf("error consultando estado de P1: %v", err)
	}
	if status1.Status != domain.StatusQueued {
		t.Fatalf("P1 debería estar QUEUED, se obtuvo: %v", status1.Status)
	}

	// Jugador 2 y 3 se encolan
	res2, err := enqueueUC.Execute(ctx, "trace-2", p2, "2v2")
	if err != nil || res2.Status != domain.StatusQueued {
		t.Fatalf("error encolando jugador 2: %v", err)
	}

	res3, err := enqueueUC.Execute(ctx, "trace-3", p3, "2v2")
	if err != nil || res3.Status != domain.StatusQueued {
		t.Fatalf("error encolando jugador 3: %v", err)
	}

	// Jugador 4 se encola -> debe armarse la partida 2v2 inmediatamente
	res4, err := enqueueUC.Execute(ctx, "trace-4", p4, "2v2")
	if err != nil {
		t.Fatalf("error encolando jugador 4: %v", err)
	}
	if res4.Status != domain.StatusMatchFound {
		t.Fatalf("se esperaba estado MATCH_FOUND para jugador 4, se obtuvo: %v", res4.Status)
	}
	if res4.MatchID == "" || res4.JoinTicket == "" {
		t.Fatalf("se esperaba MatchID y JoinTicket poblados para jugador 4: %+v", res4)
	}
	if res4.ServerHost != "127.0.0.1" || res4.ServerPort != 7777 {
		t.Fatalf("host/puerto de servidor incorrectos: %s:%d", res4.ServerHost, res4.ServerPort)
	}

	// Ahora los primeros 3 jugadores consultan su estado vía GetStatusUseCase
	for _, pid := range []string{p1, p2, p3} {
		status, err := statusUC.Execute(ctx, "trace-check", pid)
		if err != nil {
			t.Fatalf("error consultando status de %s: %v", pid, err)
		}
		if status.Status != domain.StatusMatchFound {
			t.Fatalf("se esperaba MATCH_FOUND para %s, se obtuvo: %v", pid, status.Status)
		}
		if status.MatchID != res4.MatchID {
			t.Fatalf("todos los jugadores deben tener el mismo MatchID: %s vs %s", status.MatchID, res4.MatchID)
		}
		if status.JoinTicket == "" {
			t.Fatalf("el jugador %s debe tener un ticket asignado", pid)
		}
	}

	// Validar distribución de equipos: 2 en TEAM_A y 2 en TEAM_B
	teamACount := 0
	teamBCount := 0
	for _, pid := range []string{p1, p2, p3, p4} {
		status, _ := statusUC.Execute(ctx, "trace-team", pid)
		if status.AssignedTeam == domain.TeamSideA {
			teamACount++
		} else if status.AssignedTeam == domain.TeamSideB {
			teamBCount++
		}
	}
	if teamACount != 2 || teamBCount != 2 {
		t.Fatalf("se esperaba balance 2v2 (2 en A, 2 en B), se obtuvo Team A: %d, Team B: %d", teamACount, teamBCount)
	}
}

func TestValidateJoinTicketAndMarkStartedFlow(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockMatchRepo()

	matchID := "match-uuid-1234"
	ticketToken := "ticket-token-5678"
	playerID := "player-uuid-9999"

	repo.matches[matchID] = domain.Match{
		ID:         matchID,
		Mode:       domain.Mode2v2,
		Status:     domain.MatchStatusCreated,
		ServerHost: "127.0.0.1",
		ServerPort: 7777,
		CreatedAt:  time.Now().UTC(),
	}
	repo.tickets[ticketToken] = domain.MatchTicket{
		TicketToken:  ticketToken,
		MatchID:      matchID,
		PlayerID:     playerID,
		AssignedTeam: domain.TeamSideA,
		ExpiresAt:    time.Now().UTC().Add(10 * time.Minute),
		CreatedAt:    time.Now().UTC(),
	}

	validateUC := application.NewValidateJoinTicketUseCase(logger, repo)
	markStartedUC := application.NewMarkMatchStartedUseCase(logger, repo)

	// Validar ticket por primera vez (debe ser exitoso)
	valRes, err := validateUC.Execute(ctx, "trace-v", matchID, ticketToken)
	if err != nil {
		t.Fatalf("error inesperado validando ticket: %v", err)
	}
	if !valRes.Valid || valRes.ReasonCode != "OK" {
		t.Fatalf("se esperaba ticket válido con código OK, se obtuvo: %+v", valRes)
	}
	if valRes.PlayerID != playerID || valRes.AssignedTeam != domain.TeamSideA {
		t.Fatalf("datos de validación incorrectos: %+v", valRes)
	}

	// Reintentar validar el mismo ticket (debe fallar por ya consumido)
	valRes2, err := validateUC.Execute(ctx, "trace-v2", matchID, ticketToken)
	if err != nil {
		t.Fatalf("error inesperado en reintento: %v", err)
	}
	if valRes2.Valid || valRes2.ReasonCode != "TICKET_ALREADY_USED" {
		t.Fatalf("se esperaba rechazo TICKET_ALREADY_USED, se obtuvo: %+v", valRes2)
	}

	// Marcar partida como iniciada
	if err := markStartedUC.Execute(ctx, "trace-s", matchID, "srv-instance-1"); err != nil {
		t.Fatalf("error marcando partida iniciada: %v", err)
	}

	m := repo.matches[matchID]
	if m.Status != domain.MatchStatusInProgress || m.StartedAt == nil {
		t.Fatalf("la partida debería tener status in_progress y StartedAt poblado: %+v", m)
	}
}

func TestDequeueUseCase(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockMatchRepo()
	queue := newMockQueue()

	enqueueUC := application.NewEnqueueUseCase(logger, repo, queue, "127.0.0.1", 7777)
	dequeueUC := application.NewDequeueUseCase(logger, queue)
	statusUC := application.NewGetStatusUseCase(logger, repo, queue)

	playerID := "player-to-cancel"

	// Cancelar cuando no está en cola devuelve ErrNotInQueue
	if err := dequeueUC.Execute(ctx, "trace", playerID); err != domain.ErrNotInQueue {
		t.Fatalf("se esperaba ErrNotInQueue, se obtuvo: %v", err)
	}

	// Encolar y luego cancelar
	_, _ = enqueueUC.Execute(ctx, "trace", playerID, "2v2")
	if err := dequeueUC.Execute(ctx, "trace", playerID); err != nil {
		t.Fatalf("error cancelando cola: %v", err)
	}

	// El estado debe volver a IDLE
	st, err := statusUC.Execute(ctx, "trace", playerID)
	if err != nil {
		t.Fatalf("error consultando estado tras cancelar: %v", err)
	}
	if st.Status != domain.StatusIdle {
		t.Fatalf("se esperaba IDLE tras cancelar, se obtuvo: %v", st.Status)
	}
}
