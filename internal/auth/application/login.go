package application

import (
	"context"
	"errors"

	"server/internal/auth/domain"
	"server/kit/logging"
)

// LoginUseCase define el Puerto de Entrada (Primary Port) de la Capa de Aplicación
// para iniciar sesión o aprovisionar rápidamente una cuenta en modo desarrollo (según OpenAPI).
type LoginUseCase interface {
	Execute(ctx context.Context, traceID, username, password string) (domain.AuthSession, error)
}

type loginUseCase struct {
	log        logging.Logger
	users      domain.UserRepository
	hasher     domain.PasswordHasher
	tokenMaker domain.TokenManager
}

// NewLoginUseCase construye el caso de uso de login inyectando sus puertos del dominio.
func NewLoginUseCase(
	log logging.Logger,
	users domain.UserRepository,
	hasher domain.PasswordHasher,
	tokenMaker domain.TokenManager,
) LoginUseCase {
	return &loginUseCase{
		log:        log,
		users:      users,
		hasher:     hasher,
		tokenMaker: tokenMaker,
	}
}

// Execute valida las credenciales del jugador:
//  1. Verifica formato de username (3..32) y password (6..72).
//  2. Busca el usuario por username.
//  3. Si no existe (`ErrUserNotFound`), hashea la contraseña y crea de forma transaccional
//     el usuario, su perfil inicial (`MMR = 1000`, `level = 1`) y su billetera (`balance = 0`).
//  4. Si ya existe, compara la contraseña con el hash almacenado.
//  5. Emite y devuelve el token JWT firmado junto con `player_id` y `display_name`.
func (uc *loginUseCase) Execute(ctx context.Context, traceID, username, password string) (domain.AuthSession, error) {
	cleanUsername, err := domain.ValidateCredentials(username, password)
	if err != nil {
		uc.log.InfoContext(ctx, "login rejected due to invalid input format", "trace_id", traceID, "reason", err.Error())
		return domain.AuthSession{}, err
	}

	user, err := uc.users.FindByUsername(ctx, cleanUsername)
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		// Flujo de registro rápido en primer login (definido en contracts/openapi/openapi.yaml).
		passwordHash, hashErr := uc.hasher.Hash(password)
		if hashErr != nil {
			uc.log.ErrorContext(ctx, "failed to hash password", "trace_id", traceID, "error", hashErr)
			return domain.AuthSession{}, hashErr
		}

		user, err = uc.users.CreateAccountWithInitialProfile(ctx, cleanUsername, passwordHash)
		if err != nil {
			uc.log.ErrorContext(ctx, "failed to create user account", "trace_id", traceID, "username", cleanUsername, "error", err)
			return domain.AuthSession{}, err
		}
		uc.log.InfoContext(ctx, "new player account provisioned", "trace_id", traceID, "player_id", user.ID, "username", user.Username)

	case err != nil:
		uc.log.ErrorContext(ctx, "failed to query user by username", "trace_id", traceID, "username", cleanUsername, "error", err)
		return domain.AuthSession{}, err

	default:
		// Usuario existente: verificamos su contraseña contra el hash bcrypt guardado.
		if cmpErr := uc.hasher.Compare(user.PasswordHash, password); cmpErr != nil {
			uc.log.InfoContext(ctx, "login failed due to invalid password", "trace_id", traceID, "player_id", user.ID)
			return domain.AuthSession{}, domain.ErrInvalidCredentials
		}
	}

	accessToken, err := uc.tokenMaker.GenerateToken(user.ID, user.Username)
	if err != nil {
		uc.log.ErrorContext(ctx, "failed to generate access token", "trace_id", traceID, "player_id", user.ID, "error", err)
		return domain.AuthSession{}, err
	}

	uc.log.InfoContext(ctx, "player authenticated successfully", "trace_id", traceID, "player_id", user.ID)
	return domain.AuthSession{
		AccessToken: accessToken,
		PlayerID:    user.ID,
		DisplayName: user.DisplayName,
	}, nil
}
