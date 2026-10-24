package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"server/internal/matchmaking/domain"
)

// MatchRepository es el Adaptador Secundario de almacenamiento en PostgreSQL (pgxpool v5)
// para el agregado de Partidas y Tickets de acceso.
type MatchRepository struct {
	pool *pgxpool.Pool
}

var _ domain.MatchRepository = (*MatchRepository)(nil)

// NewMatchRepository construye el repositorio PostgreSQL de matchmaking.
func NewMatchRepository(pool *pgxpool.Pool) *MatchRepository {
	return &MatchRepository{pool: pool}
}

// CreateMatchWithTickets inserta en una única transacción ACID la partida y los tickets de los jugadores emparejados.
func (r *MatchRepository) CreateMatchWithTickets(
	ctx context.Context,
	match domain.Match,
	tickets []domain.MatchTicket,
) error {
	if r.pool == nil {
		return errors.New("el pool de conexiones de base de datos no está inicializado")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error al iniciar transacción para crear partida: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const insertMatchSQL = `
		INSERT INTO matches (
			id,
			mode,
			status,
			server_host,
			server_port,
			created_at
		) VALUES (
			$1::uuid,
			$2,
			$3,
			$4,
			$5,
			$6
		)
	`
	_, err = tx.Exec(
		ctx,
		insertMatchSQL,
		match.ID,
		string(match.Mode),
		string(match.Status),
		match.ServerHost,
		match.ServerPort,
		match.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("error insertando partida %q: %w", match.ID, err)
	}

	const insertTicketSQL = `
		INSERT INTO match_tickets (
			ticket_token,
			match_id,
			user_id,
			assigned_team,
			expires_at,
			created_at
		) VALUES (
			$1,
			$2::uuid,
			$3::uuid,
			$4,
			$5,
			$6
		)
	`
	for _, t := range tickets {
		_, err = tx.Exec(
			ctx,
			insertTicketSQL,
			t.TicketToken,
			t.MatchID,
			t.PlayerID,
			string(t.AssignedTeam),
			t.ExpiresAt,
			t.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("error insertando ticket para el jugador %q: %w", t.PlayerID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error al confirmar la transacción de creación de partida: %w", err)
	}

	return nil
}

// GetMatchByID recupera una partida por su identificador UUID.
func (r *MatchRepository) GetMatchByID(ctx context.Context, matchID string) (domain.Match, error) {
	if r.pool == nil {
		return domain.Match{}, errors.New("el pool de conexiones de base de datos no está inicializado")
	}

	const query = `
		SELECT
			id::text,
			mode,
			status,
			COALESCE(outcome, '') AS outcome,
			total_rounds,
			ended_in_duel,
			duration_seconds,
			server_host,
			server_port,
			created_at,
			started_at,
			ended_at
		FROM matches
		WHERE id = $1::uuid
	`

	var m domain.Match
	var modeStr, statusStr string

	err := r.pool.QueryRow(ctx, query, matchID).Scan(
		&m.ID,
		&modeStr,
		&statusStr,
		&m.Outcome,
		&m.TotalRounds,
		&m.EndedInDuel,
		&m.DurationSeconds,
		&m.ServerHost,
		&m.ServerPort,
		&m.CreatedAt,
		&m.StartedAt,
		&m.EndedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Match{}, domain.ErrMatchNotFound
		}
		return domain.Match{}, fmt.Errorf("error consultando partida %q: %w", matchID, err)
	}

	m.Mode = domain.GameMode(modeStr)
	m.Status = domain.MatchStatus(statusStr)
	return m, nil
}

// FindActiveTicketByPlayerID busca un ticket vigente y no consumido para el jugador en una partida activa.
func (r *MatchRepository) FindActiveTicketByPlayerID(
	ctx context.Context,
	playerID string,
) (*domain.MatchTicket, *domain.Match, error) {
	if r.pool == nil {
		return nil, nil, errors.New("el pool de conexiones de base de datos no está inicializado")
	}

	const query = `
		SELECT
			t.ticket_token,
			t.match_id::text,
			t.user_id::text,
			t.assigned_team,
			t.expires_at,
			t.consumed_at,
			t.created_at,
			m.id::text,
			m.mode,
			m.status,
			m.server_host,
			m.server_port,
			m.created_at
		FROM match_tickets t
		JOIN matches m ON m.id = t.match_id
		WHERE t.user_id = $1::uuid
		  AND t.consumed_at IS NULL
		  AND t.expires_at > now()
		  AND m.status IN ('created', 'in_progress')
		ORDER BY t.created_at DESC
		LIMIT 1
	`

	var t domain.MatchTicket
	var m domain.Match
	var teamStr, modeStr, statusStr string

	err := r.pool.QueryRow(ctx, query, playerID).Scan(
		&t.TicketToken,
		&t.MatchID,
		&t.PlayerID,
		&teamStr,
		&t.ExpiresAt,
		&t.ConsumedAt,
		&t.CreatedAt,
		&m.ID,
		&modeStr,
		&statusStr,
		&m.ServerHost,
		&m.ServerPort,
		&m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("error buscando ticket activo para el jugador %q: %w", playerID, err)
	}

	t.AssignedTeam = domain.TeamSide(teamStr)
	m.Mode = domain.GameMode(modeStr)
	m.Status = domain.MatchStatus(statusStr)

	return &t, &m, nil
}

// ValidateAndConsumeTicket verifica la autenticidad del ticket, comprueba que corresponda a la partida,
// que no haya expirado ni sido consumido previamente, lo marca como consumido y consulta los datos del jugador.
func (r *MatchRepository) ValidateAndConsumeTicket(
	ctx context.Context,
	matchID, ticketToken string,
) (domain.ValidatedTicket, error) {
	if r.pool == nil {
		return domain.ValidatedTicket{Valid: false, ReasonCode: "DATABASE_UNAVAILABLE"}, errors.New("el pool de conexiones de base de datos no está inicializado")
	}

	const selectSQL = `
		SELECT
			t.ticket_token,
			t.match_id::text,
			t.user_id::text,
			t.assigned_team,
			t.expires_at,
			t.consumed_at,
			m.status,
			p.display_name,
			p.equipped_skin_code,
			p.equipped_death_effect_code,
			p.mmr
		FROM match_tickets t
		JOIN matches m ON m.id = t.match_id
		JOIN player_profiles p ON p.user_id = t.user_id
		WHERE t.ticket_token = $1
	`

	var token, matchIDFromDB, userID, assignedTeamStr, matchStatus, displayName, skinCode, deathEffectCode string
	var expiresAt time.Time
	var consumedAt *time.Time
	var mmr int

	err := r.pool.QueryRow(ctx, selectSQL, ticketToken).Scan(
		&token,
		&matchIDFromDB,
		&userID,
		&assignedTeamStr,
		&expiresAt,
		&consumedAt,
		&matchStatus,
		&displayName,
		&skinCode,
		&deathEffectCode,
		&mmr,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ValidatedTicket{
				Valid:      false,
				ReasonCode: "TICKET_NOT_FOUND",
			}, nil
		}
		return domain.ValidatedTicket{Valid: false, ReasonCode: "QUERY_ERROR"}, fmt.Errorf("error consultando ticket %q: %w", ticketToken, err)
	}

	// 1. Verificación de match_id
	if matchIDFromDB != matchID {
		return domain.ValidatedTicket{
			Valid:      false,
			ReasonCode: "MATCH_MISMATCH",
		}, nil
	}

	// 2. Verificación de uso previo (unicidad de uso)
	if consumedAt != nil {
		return domain.ValidatedTicket{
			Valid:      false,
			ReasonCode: "TICKET_ALREADY_USED",
		}, nil
	}

	// 3. Verificación de caducidad temporal
	if time.Now().UTC().After(expiresAt) {
		return domain.ValidatedTicket{
			Valid:      false,
			ReasonCode: "TICKET_EXPIRED",
		}, nil
	}

	// 4. Verificación del estado de la partida
	if matchStatus == string(domain.MatchStatusCancelled) || matchStatus == string(domain.MatchStatusCompleted) {
		return domain.ValidatedTicket{
			Valid:      false,
			ReasonCode: "MATCH_NOT_ACTIVE",
		}, nil
	}

	// 5. Consumo atómico del ticket para impedir reutilización concurrente
	const updateTicketSQL = `
		UPDATE match_tickets
		SET consumed_at = now()
		WHERE ticket_token = $1 AND consumed_at IS NULL
	`
	cmdTag, err := r.pool.Exec(ctx, updateTicketSQL, ticketToken)
	if err != nil {
		return domain.ValidatedTicket{Valid: false, ReasonCode: "UPDATE_ERROR"}, fmt.Errorf("error consumiendo ticket %q: %w", ticketToken, err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domain.ValidatedTicket{
			Valid:      false,
			ReasonCode: "TICKET_ALREADY_USED",
		}, nil
	}

	return domain.ValidatedTicket{
		Valid:                   true,
		ReasonCode:              "OK",
		PlayerID:                userID,
		DisplayName:             displayName,
		AssignedTeam:            domain.TeamSide(assignedTeamStr),
		EquippedSkinCode:        skinCode,
		EquippedDeathEffectCode: deathEffectCode,
		CurrentMMR:              mmr,
	}, nil
}

// MarkMatchStarted transiciona la partida de 'created' a 'in_progress' y registra el started_at.
func (r *MatchRepository) MarkMatchStarted(ctx context.Context, matchID string) error {
	if r.pool == nil {
		return errors.New("el pool de conexiones de base de datos no está inicializado")
	}

	const query = `
		UPDATE matches
		SET status = 'in_progress',
		    started_at = COALESCE(started_at, now())
		WHERE id = $1::uuid AND status = 'created'
	`

	cmdTag, err := r.pool.Exec(ctx, query, matchID)
	if err != nil {
		return fmt.Errorf("error actualizando partida %q a in_progress: %w", matchID, err)
	}

	if cmdTag.RowsAffected() == 0 {
		// Verificamos si la partida ya estaba in_progress (idempotencia)
		var status string
		err := r.pool.QueryRow(ctx, "SELECT status FROM matches WHERE id = $1::uuid", matchID).Scan(&status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrMatchNotFound
			}
			return fmt.Errorf("error verificando estado de partida %q: %w", matchID, err)
		}
		if status == string(domain.MatchStatusInProgress) {
			return nil
		}
		return domain.ErrMatchNotActive
	}

	return nil
}
