package domain

import (
	"context"
	"strings"
	"time"
)

const (
	// MinUsernameLength y MaxUsernameLength reflejan las restricciones del contrato OpenAPI (/v1/auth/login).
	MinUsernameLength = 3
	MaxUsernameLength = 32

	// MinPasswordLength y MaxPasswordLength reflejan las restricciones del contrato OpenAPI y el límite de 72 bytes de bcrypt.
	MinPasswordLength = 6
	MaxPasswordLength = 72
)

// User es la Entidad Raíz (Aggregate Root) del Bounded Context de Autenticación.
// Representa una cuenta de jugador junto con su nombre visible inicial.
type User struct {
	ID           string
	Username     string
	DisplayName  string
	PasswordHash string
	CreatedAt    time.Time
}

// AuthSession es el Value Object devuelto tras una autenticación o registro exitoso.
type AuthSession struct {
	AccessToken string
	PlayerID    string
	DisplayName string
}

// ValidateCredentials aplica las invariantes de negocio sobre el nombre de usuario y contraseña
// antes de consultar la base de datos o ejecutar el algoritmo de hashing.
func ValidateCredentials(username, password string) (string, error) {
	cleanUsername := strings.TrimSpace(username)
	if len(cleanUsername) < MinUsernameLength || len(cleanUsername) > MaxUsernameLength {
		return "", ErrInvalidUsername
	}
	if len(password) < MinPasswordLength || len(password) > MaxPasswordLength {
		return "", ErrInvalidPassword
	}
	return cleanUsername, nil
}

// UserRepository es el Puerto de Salida (Secondary Port) para la persistencia de cuentas.
// La implementación concreta en PostgreSQL (infrastructure/storage/postgres) se encarga de que
// CreateAccountWithInitialProfile ejecute una transacción ACID creando usuario, perfil (MMR 1000),
// billetera y cosméticos iniciales por defecto.
type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (User, error)
	CreateAccountWithInitialProfile(ctx context.Context, username, passwordHash string) (User, error)
}

// PasswordHasher es el Puerto de Salida para hashear y verificar contraseñas (ej. bcrypt).
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(passwordHash, plainPassword string) error
}

// TokenManager es el Puerto de Salida para emitir y validar tokens de acceso (JWT).
type TokenManager interface {
	GenerateToken(userID, username string) (string, error)
	ValidateToken(tokenString string) (userID string, err error)
}
