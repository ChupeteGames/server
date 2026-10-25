package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"server/internal/auth/infrastructure/security"
	authhandlers "server/internal/auth/infrastructure/transport/http/handlers"
	healthapp "server/internal/health/application"
	healthhttp "server/internal/health/infrastructure/transport/http"
	matchmakingapp "server/internal/matchmaking/application"
	matchmakingdomain "server/internal/matchmaking/domain"
	memoryqueue "server/internal/matchmaking/infrastructure/queue/memory"
	matchmakinghttp "server/internal/matchmaking/infrastructure/transport/http"
)

type inMemoryMatchRepository struct {
	mu      sync.Mutex
	matches map[string]matchmakingdomain.Match
	tickets map[string]matchmakingdomain.MatchTicket
}

func newInMemoryMatchRepository() *inMemoryMatchRepository {
	return &inMemoryMatchRepository{
		matches: make(map[string]matchmakingdomain.Match),
		tickets: make(map[string]matchmakingdomain.MatchTicket),
	}
}

func (r *inMemoryMatchRepository) CreateMatchWithTickets(
	_ context.Context,
	match matchmakingdomain.Match,
	tickets []matchmakingdomain.MatchTicket,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.matches[match.ID] = match
	for _, t := range tickets {
		r.tickets[t.TicketToken] = t
	}
	return nil
}

func (r *inMemoryMatchRepository) GetMatchByID(_ context.Context, matchID string) (matchmakingdomain.Match, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.matches[matchID]
	if !ok {
		return matchmakingdomain.Match{}, matchmakingdomain.ErrMatchNotFound
	}
	return m, nil
}

func (r *inMemoryMatchRepository) FindActiveTicketByPlayerID(
	_ context.Context,
	playerID string,
) (*matchmakingdomain.MatchTicket, *matchmakingdomain.Match, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tickets {
		if t.PlayerID == playerID && t.ConsumedAt == nil && time.Now().UTC().Before(t.ExpiresAt) {
			m := r.matches[t.MatchID]
			tCopy := t
			return &tCopy, &m, nil
		}
	}
	return nil, nil, nil
}

func (r *inMemoryMatchRepository) ValidateAndConsumeTicket(
	_ context.Context,
	matchID, ticketToken string,
) (matchmakingdomain.ValidatedTicket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ticket, ok := r.tickets[ticketToken]
	if !ok {
		return matchmakingdomain.ValidatedTicket{Valid: false, ReasonCode: "TICKET_NOT_FOUND"}, nil
	}
	if ticket.MatchID != matchID {
		return matchmakingdomain.ValidatedTicket{Valid: false, ReasonCode: "MATCH_MISMATCH"}, nil
	}
	if ticket.ConsumedAt != nil {
		return matchmakingdomain.ValidatedTicket{Valid: false, ReasonCode: "TICKET_ALREADY_USED"}, nil
	}
	if time.Now().UTC().After(ticket.ExpiresAt) {
		return matchmakingdomain.ValidatedTicket{Valid: false, ReasonCode: "TICKET_EXPIRED"}, nil
	}

	now := time.Now().UTC()
	ticket.ConsumedAt = &now
	r.tickets[ticketToken] = ticket

	return matchmakingdomain.ValidatedTicket{
		Valid:                   true,
		ReasonCode:              "OK",
		PlayerID:                ticket.PlayerID,
		DisplayName:             "Jugador",
		AssignedTeam:            ticket.AssignedTeam,
		EquippedSkinCode:        "bean_default",
		EquippedDeathEffectCode: "gore_classic",
		CurrentMMR:              1000,
	}, nil
}

func (r *inMemoryMatchRepository) MarkMatchStarted(_ context.Context, matchID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.matches[matchID]
	if !ok {
		return matchmakingdomain.ErrMatchNotFound
	}
	m.Status = matchmakingdomain.MatchStatusInProgress
	now := time.Now().UTC()
	m.StartedAt = &now
	r.matches[matchID] = m
	return nil
}

type matchResponseDTO struct {
	Status       string  `json:"status"`
	Mode         string  `json:"mode"`
	MatchID      string  `json:"match_id"`
	ServerHost   string  `json:"server_host"`
	ServerPort   int     `json:"server_port"`
	JoinTicket   string  `json:"join_ticket"`
	AssignedTeam string  `json:"assigned_team"`
	ExpiresAt    *string `json:"expires_at"`
}

func setupTestRouter(
	logger *slog.Logger,
	jwtMgr *security.JWTManager,
	repo matchmakingdomain.MatchRepository,
	queue matchmakingdomain.MatchmakingQueue,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := healthhttp.NewRouter(logger, healthapp.NewHealthUseCase(logger))
	authMiddleware := authhandlers.BearerAuthMiddleware(logger, jwtMgr)

	enqueueUC := matchmakingapp.NewEnqueueUseCase(logger, repo, queue, "127.0.0.1", 7777)
	statusUC := matchmakingapp.NewGetStatusUseCase(logger, repo, queue)
	dequeueUC := matchmakingapp.NewDequeueUseCase(logger, queue)
	validateTicketUC := matchmakingapp.NewValidateJoinTicketUseCase(logger, repo)
	markStartedUC := matchmakingapp.NewMarkMatchStartedUseCase(logger, repo)

	matchmakinghttp.RegisterRoutes(router, logger, authMiddleware, enqueueUC, statusUC, dequeueUC)
	matchmakinghttp.RegisterDedicatedServerRoutes(router, logger, validateTicketUC, markStartedUC)
	return router
}

func TestMatchmakingHTTPEndToEndFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtMgr := security.NewJWTManager("test_secret_for_matchmaking_http_tests_32b")
	repo := newInMemoryMatchRepository()
	queue := memoryqueue.NewMemoryQueue()

	router := setupTestRouter(logger, jwtMgr, repo, queue)

	players := []struct {
		id       string
		username string
		token    string
	}{
		{id: "11111111-0000-0000-0000-000000000001", username: "jugador1"},
		{id: "22222222-0000-0000-0000-000000000002", username: "jugador2"},
		{id: "33333333-0000-0000-0000-000000000003", username: "jugador3"},
		{id: "44444444-0000-0000-0000-000000000004", username: "jugador4"},
	}

	for i := range players {
		token, err := jwtMgr.GenerateToken(players[i].id, players[i].username)
		if err != nil {
			t.Fatalf("error generando token para jugador %d: %v", i+1, err)
		}
		players[i].token = token
	}

	// 1. Probar que sin token devuelve 401 Unauthorized
	reqUnauthorized, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{"mode":"2v2"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, reqUnauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 sin header Bearer, se obtuvo %d", rec.Code)
	}

	// 2. Probar que con modo inválido devuelve 400 Bad Request
	reqInvalidMode, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{"mode":"10v10"}`))
	reqInvalidMode.Header.Set("Authorization", "Bearer "+players[0].token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqInvalidMode)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("se esperaba 400 con modo inválido, se obtuvo %d", rec.Code)
	}

	// 3. Jugador 1 se encola en 2v2
	reqP1, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{"mode":"2v2"}`))
	reqP1.Header.Set("Authorization", "Bearer "+players[0].token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqP1)
	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 al encolar P1, se obtuvo %d: %s", rec.Code, rec.Body.String())
	}
	var respP1 matchResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &respP1); err != nil {
		t.Fatalf("error parseando respuesta: %v", err)
	}
	if respP1.Status != "QUEUED" {
		t.Fatalf("se esperaba status QUEUED para P1, se obtuvo: %s", respP1.Status)
	}

	// 4. Jugador 1 intenta doble encolado -> debe dar 409 Conflict
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqP1)
	if rec.Code != http.StatusConflict {
		t.Fatalf("se esperaba 409 en doble encolado, se obtuvo %d", rec.Code)
	}

	// 5. Jugador 1 consulta su status -> debe reportar QUEUED
	reqStatusP1, _ := http.NewRequest(http.MethodGet, "/v1/matchmaking/status", nil)
	reqStatusP1.Header.Set("Authorization", "Bearer "+players[0].token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqStatusP1)
	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 en status P1, se obtuvo %d", rec.Code)
	}
	var statusP1 matchResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &statusP1)
	if statusP1.Status != "QUEUED" {
		t.Fatalf("se esperaba status QUEUED, se obtuvo %s", statusP1.Status)
	}

	// 6. Encolar Jugador 2 y Jugador 3
	for i := 1; i <= 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", "Bearer "+players[i].token)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("se esperaba 200 al encolar jugador %d, se obtuvo %d", i+1, rec.Code)
		}
		var r matchResponseDTO
		_ = json.Unmarshal(rec.Body.Bytes(), &r)
		if r.Status != "QUEUED" {
			t.Fatalf("se esperaba QUEUED para jugador %d, se obtuvo %s", i+1, r.Status)
		}
	}

	// 7. Encolar Jugador 4 -> Criterio de Aceptación MVP-008: ¡Se conforma la partida 2v2!
	reqP4, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{"mode":"2v2"}`))
	reqP4.Header.Set("Authorization", "Bearer "+players[3].token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqP4)
	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 al encolar P4, se obtuvo %d: %s", rec.Code, rec.Body.String())
	}
	var respP4 matchResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &respP4); err != nil {
		t.Fatalf("error parseando respuesta P4: %v", err)
	}
	if respP4.Status != "MATCH_FOUND" {
		t.Fatalf("se esperaba MATCH_FOUND al encolar el 4to jugador, se obtuvo: %s", respP4.Status)
	}
	if respP4.MatchID == "" || respP4.JoinTicket == "" {
		t.Fatalf("P4 debería tener match_id y join_ticket válidos: %+v", respP4)
	}
	if respP4.ServerHost != "127.0.0.1" || respP4.ServerPort != 7777 {
		t.Fatalf("servidor dedicado inesperado: %s:%d", respP4.ServerHost, respP4.ServerPort)
	}

	// 8. Verificar que los jugadores 1, 2 y 3 al consultar GET /v1/matchmaking/status reciben su MATCH_FOUND
	assignedTeams := make(map[string]int)
	assignedTeams[respP4.AssignedTeam]++

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/v1/matchmaking/status", nil)
		req.Header.Set("Authorization", "Bearer "+players[i].token)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("error consultando status de jugador %d: %d", i+1, rec.Code)
		}
		var st matchResponseDTO
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		if st.Status != "MATCH_FOUND" {
			t.Fatalf("jugador %d debería tener MATCH_FOUND, se obtuvo %s", i+1, st.Status)
		}
		if st.MatchID != respP4.MatchID {
			t.Fatalf("jugador %d tiene match_id distinto: %s vs %s", i+1, st.MatchID, respP4.MatchID)
		}
		if st.JoinTicket == "" {
			t.Fatalf("jugador %d debería tener un join_ticket no vacío", i+1)
		}
		assignedTeams[st.AssignedTeam]++
	}

	// 9. Verificar balance de equipos: exactamente 2 en TEAM_A y 2 en TEAM_B
	if assignedTeams["TEAM_A"] != 2 || assignedTeams["TEAM_B"] != 2 {
		t.Fatalf("los equipos deben estar balanceados 2v2 (2 TEAM_A, 2 TEAM_B), conteo: %+v", assignedTeams)
	}
}

func TestMatchmakingCancelFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtMgr := security.NewJWTManager("test_secret_for_matchmaking_cancel_tests_32b")
	repo := newInMemoryMatchRepository()
	queue := memoryqueue.NewMemoryQueue()

	router := setupTestRouter(logger, jwtMgr, repo, queue)

	token, _ := jwtMgr.GenerateToken("player-cancel-123", "cancelUser")

	// Cancelar cuando no está en cola -> 404
	reqCancel, _ := http.NewRequest(http.MethodDelete, "/v1/matchmaking/dequeue", nil)
	reqCancel.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, reqCancel)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("se esperaba 404 al cancelar sin estar en cola, se obtuvo %d", rec.Code)
	}

	// Encolar
	reqEnqueue, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{"mode":"2v2"}`))
	reqEnqueue.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqEnqueue)
	if rec.Code != http.StatusOK {
		t.Fatalf("error al encolar: %d", rec.Code)
	}

	// Cancelar exitosamente -> 200
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqCancel)
	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 al cancelar cola, se obtuvo %d", rec.Code)
	}

	// Consultar status -> IDLE
	reqStatus, _ := http.NewRequest(http.MethodGet, "/v1/matchmaking/status", nil)
	reqStatus.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, reqStatus)
	var st matchResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Status != "IDLE" {
		t.Fatalf("se esperaba status IDLE tras cancelar, se obtuvo %s", st.Status)
	}
}

func TestDedicatedServerHTTPValidateTicketAndMarkStarted(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtMgr := security.NewJWTManager("test_secret_for_dedicated_server_http_32b")
	repo := newInMemoryMatchRepository()
	queue := memoryqueue.NewMemoryQueue()

	router := setupTestRouter(logger, jwtMgr, repo, queue)

	// 1. Armar una partida 2v2 con 4 jugadores
	var lastMatch matchResponseDTO
	for i := 1; i <= 4; i++ {
		playerID := "00000000-0000-0000-0000-00000000000" + string(rune('0'+i))
		token, _ := jwtMgr.GenerateToken(playerID, "unreal_player")
		req, _ := http.NewRequest(http.MethodPost, "/v1/matchmaking/enqueue", bytes.NewBufferString(`{"mode":"2v2"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("error encolando jugador %d: %d", i, rec.Code)
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &lastMatch)
	}

	if lastMatch.Status != "MATCH_FOUND" || lastMatch.MatchID == "" || lastMatch.JoinTicket == "" {
		t.Fatalf("se esperaba MATCH_FOUND con match_id y join_ticket: %+v", lastMatch)
	}

	// 2. El Unreal Dedicated Server valida el ticket por HTTP (POST /v1/internal/matches/validate-ticket)
	validatePayload := `{"trace_id":"unreal-ds-trace-001","match_id":"` + lastMatch.MatchID + `","join_ticket":"` + lastMatch.JoinTicket + `"}`
	reqValidate, _ := http.NewRequest(http.MethodPost, "/v1/internal/matches/validate-ticket", bytes.NewBufferString(validatePayload))
	reqValidate.Header.Set("Content-Type", "application/json")
	recValidate := httptest.NewRecorder()
	router.ServeHTTP(recValidate, reqValidate)

	if recValidate.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 OK en validate-ticket, se obtuvo %d: %s", recValidate.Code, recValidate.Body.String())
	}
	if gotTrace := recValidate.Header().Get("X-Trace-ID"); gotTrace != "unreal-ds-trace-001" {
		t.Fatalf("se esperaba X-Trace-ID 'unreal-ds-trace-001', se obtuvo %q", gotTrace)
	}

	var valResp struct {
		Valid            bool   `json:"valid"`
		ReasonCode       string `json:"reason_code"`
		PlayerID         string `json:"player_id"`
		AssignedTeam     string `json:"assigned_team"`
		EquippedSkinCode string `json:"equipped_skin_code"`
	}
	if err := json.Unmarshal(recValidate.Body.Bytes(), &valResp); err != nil {
		t.Fatalf("error parseando respuesta de validate-ticket: %v", err)
	}
	if !valResp.Valid || valResp.ReasonCode != "OK" {
		t.Fatalf("se esperaba ticket válido con OK, se obtuvo: %+v", valResp)
	}
	if valResp.EquippedSkinCode != "bean_default" {
		t.Fatalf("se esperaba skin 'bean_default', se obtuvo %q", valResp.EquippedSkinCode)
	}

	// 3. Reintento con el mismo ticket debe ser rechazado con TICKET_ALREADY_USED
	reqValidateDup, _ := http.NewRequest(http.MethodPost, "/v1/internal/matches/validate-ticket", bytes.NewBufferString(validatePayload))
	reqValidateDup.Header.Set("Content-Type", "application/json")
	recDup := httptest.NewRecorder()
	router.ServeHTTP(recDup, reqValidateDup)
	var dupResp struct {
		Valid      bool   `json:"valid"`
		ReasonCode string `json:"reason_code"`
	}
	_ = json.Unmarshal(recDup.Body.Bytes(), &dupResp)
	if dupResp.Valid || dupResp.ReasonCode != "TICKET_ALREADY_USED" {
		t.Fatalf("se esperaba TICKET_ALREADY_USED en segundo intento, se obtuvo: %+v", dupResp)
	}

	// 4. El Unreal Dedicated Server notifica el inicio de la Ronda 1 (POST /v1/internal/matches/mark-started)
	startPayload := `{"trace_id":"unreal-ds-trace-002","match_id":"` + lastMatch.MatchID + `","server_instance_id":"unreal-pie-local-1"}`
	reqStart, _ := http.NewRequest(http.MethodPost, "/v1/internal/matches/mark-started", bytes.NewBufferString(startPayload))
	reqStart.Header.Set("Content-Type", "application/json")
	recStart := httptest.NewRecorder()
	router.ServeHTTP(recStart, reqStart)

	if recStart.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 OK en mark-started, se obtuvo %d: %s", recStart.Code, recStart.Body.String())
	}
	var startResp struct {
		Success    bool   `json:"success"`
		ResultCode string `json:"result_code"`
	}
	_ = json.Unmarshal(recStart.Body.Bytes(), &startResp)
	if !startResp.Success || startResp.ResultCode != "MATCH_STARTED" {
		t.Fatalf("se esperaba MATCH_STARTED, se obtuvo: %+v", startResp)
	}
}
