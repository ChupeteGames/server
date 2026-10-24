package postgres_test

import (
	"context"
	"testing"
	"time"

	healthpostgres "server/internal/health/infrastructure/storage/postgres"
	matchmakingdomain "server/internal/matchmaking/domain"
	matchmakingpostgres "server/internal/matchmaking/infrastructure/storage/postgres"
)

func TestPostgresIntegrationMatchmaking(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	const defaultTestDB = "postgres://game_user:local_dev_only_change_me@localhost:5432/prediction_fps?sslmode=disable"
	pool, err := healthpostgres.NewPool(ctx, defaultTestDB)
	if err != nil {
		t.Skipf("saltando test de integración Postgres para matchmaking (BD local no disponible): %v", err)
		return
	}
	defer pool.Close()

	repo := matchmakingpostgres.NewMatchRepository(pool)

	// Verificamos conectividad ejecutando una consulta simple
	var exists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'matches')").Scan(&exists)
	if err != nil || !exists {
		t.Skipf("saltando test: tabla matches no existe o no accesible: %v", err)
		return
	}

	matchID := matchmakingdomain.GenerateTicketToken()
	match := matchmakingdomain.Match{
		ID:         matchID,
		Mode:       matchmakingdomain.Mode2v2,
		Status:     matchmakingdomain.MatchStatusCreated,
		ServerHost: "127.0.0.1",
		ServerPort: 7777,
		CreatedAt:  time.Now().UTC(),
	}

	// Insertar partida
	err = repo.CreateMatchWithTickets(ctx, match, nil)
	if err != nil {
		t.Fatalf("error creando partida de prueba: %v", err)
	}

	// Consultar por ID
	found, err := repo.GetMatchByID(ctx, matchID)
	if err != nil {
		t.Fatalf("error recuperando partida por ID: %v", err)
	}
	if found.ID != matchID || found.Mode != matchmakingdomain.Mode2v2 || found.Status != matchmakingdomain.MatchStatusCreated {
		t.Fatalf("datos de partida incorrectos: %+v", found)
	}

	// Marcar iniciada
	if err := repo.MarkMatchStarted(ctx, matchID); err != nil {
		t.Fatalf("error marcando partida como iniciada: %v", err)
	}

	started, err := repo.GetMatchByID(ctx, matchID)
	if err != nil {
		t.Fatalf("error consultando partida tras iniciar: %v", err)
	}
	if started.Status != matchmakingdomain.MatchStatusInProgress {
		t.Fatalf("se esperaba estado in_progress, se obtuvo: %s", started.Status)
	}
}
