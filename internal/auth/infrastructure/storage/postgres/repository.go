package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"server/internal/auth/domain"
)

// UserRepository es el Adaptador Secundario de PostgreSQL que implementa domain.UserRepository.
type UserRepository struct {
	pool *pgxpool.Pool
}

var _ domain.UserRepository = (*UserRepository)(nil)

// NewUserRepository construye el repositorio PostgreSQL para el contexto de Autenticación.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// FindByUsername busca un usuario por su nombre de cuenta junto con su display_name de perfil.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (domain.User, error) {
	const query = `
		SELECT u.id::text, u.username, COALESCE(p.display_name, u.username), u.password_hash, u.created_at
		FROM users u
		LEFT JOIN player_profiles p ON p.user_id = u.id
		WHERE u.username = $1
	`

	var user domain.User
	err := r.pool.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("error consultando usuario %q: %w", username, err)
	}

	return user, nil
}

// CreateAccountWithInitialProfile ejecuta una transacción ACID en PostgreSQL (BEGIN ... COMMIT)
// para aprovisionar de forma atómica:
//  1. La fila en `users` (credenciales).
//  2. La fila en `player_profiles` (nivel 1, XP 0, MMR 1000, skin 'bean_default' y efecto 'gore_classic').
//  3. La fila en `wallets` (billetera inicial con saldo 0).
//  4. Los cosméticos gratuitos por defecto en `player_cosmetics` ('bean_default' y 'gore_classic').
func (r *UserRepository) CreateAccountWithInitialProfile(ctx context.Context, username, passwordHash string) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("no se pudo iniciar transacción de registro: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var user domain.User
	const insertUserSQL = `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id::text, username, password_hash, created_at
	`
	if err := tx.QueryRow(ctx, insertUserSQL, username, passwordHash).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
	); err != nil {
		return domain.User{}, fmt.Errorf("no se pudo insertar usuario: %w", err)
	}
	user.DisplayName = user.Username

	const insertProfileSQL = `
		INSERT INTO player_profiles (user_id, display_name, level, xp, mmr, equipped_skin_code, equipped_death_effect_code)
		VALUES ($1::uuid, $2, 1, 0, 1000, 'bean_default', 'gore_classic')
	`
	if _, err := tx.Exec(ctx, insertProfileSQL, user.ID, user.DisplayName); err != nil {
		return domain.User{}, fmt.Errorf("no se pudo crear el perfil inicial del jugador: %w", err)
	}

	const insertWalletSQL = `
		INSERT INTO wallets (user_id, balance)
		VALUES ($1::uuid, 0)
	`
	if _, err := tx.Exec(ctx, insertWalletSQL, user.ID); err != nil {
		return domain.User{}, fmt.Errorf("no se pudo crear la billetera inicial del jugador: %w", err)
	}

	// Desbloqueamos en el inventario del jugador los cosméticos marcados como is_default = true.
	const grantDefaultCosmeticsSQL = `
		INSERT INTO player_cosmetics (user_id, cosmetic_code)
		SELECT $1::uuid, code FROM cosmetic_catalog WHERE is_default = true
		ON CONFLICT (user_id, cosmetic_code) DO NOTHING
	`
	if _, err := tx.Exec(ctx, grantDefaultCosmeticsSQL, user.ID); err != nil {
		return domain.User{}, fmt.Errorf("no se pudieron asignar los cosméticos por defecto: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("no se pudo confirmar la transacción de registro: %w", err)
	}

	return user, nil
}
