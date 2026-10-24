package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"server/internal/players/domain"
)

// PlayerRepository es el Adaptador Secundario de PostgreSQL que implementa domain.PlayerRepository.
type PlayerRepository struct {
	pool *pgxpool.Pool
}

var _ domain.PlayerRepository = (*PlayerRepository)(nil)

// NewPlayerRepository construye el repositorio PostgreSQL para el contexto de Jugadores.
func NewPlayerRepository(pool *pgxpool.Pool) *PlayerRepository {
	return &PlayerRepository{pool: pool}
}

// GetProfileByID recupera el perfil competitivo, saldo de billetera, estadísticas de carrera
// e inventario de cosméticos desbloqueados del jugador en una única lectura contra PostgreSQL.
func (r *PlayerRepository) GetProfileByID(ctx context.Context, playerID string) (domain.PlayerProfile, error) {
	const query = `
		SELECT
			p.user_id::text,
			p.display_name,
			p.level,
			p.xp,
			p.mmr,
			COALESCE(w.balance, 0) AS wallet_balance,
			p.equipped_skin_code,
			p.equipped_death_effect_code,
			p.matches_played,
			p.matches_won,
			p.matches_drawn,
			p.total_kills,
			p.total_deaths,
			p.mutual_kills,
			p.duels_won,
			COALESCE(
				ARRAY(
					SELECT pc.cosmetic_code
					FROM player_cosmetics pc
					WHERE pc.user_id = p.user_id
					ORDER BY pc.acquired_at ASC, pc.cosmetic_code ASC
				),
				ARRAY[]::text[]
			) AS owned_cosmetics
		FROM player_profiles p
		LEFT JOIN wallets w ON w.user_id = p.user_id
		WHERE p.user_id = $1::uuid
	`

	var profile domain.PlayerProfile
	err := r.pool.QueryRow(ctx, query, playerID).Scan(
		&profile.PlayerID,
		&profile.DisplayName,
		&profile.Level,
		&profile.XP,
		&profile.MMR,
		&profile.WalletBalance,
		&profile.EquippedSkinCode,
		&profile.EquippedDeathEffectCode,
		&profile.CareerStats.MatchesPlayed,
		&profile.CareerStats.MatchesWon,
		&profile.CareerStats.MatchesDrawn,
		&profile.CareerStats.TotalKills,
		&profile.CareerStats.TotalDeaths,
		&profile.CareerStats.MutualKills,
		&profile.CareerStats.DuelsWon,
		&profile.OwnedCosmetics,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PlayerProfile{}, domain.ErrPlayerNotFound
		}
		return domain.PlayerProfile{}, fmt.Errorf("error consultando perfil del jugador %q: %w", playerID, err)
	}

	if profile.OwnedCosmetics == nil {
		profile.OwnedCosmetics = []string{}
	}

	return profile, nil
}
