package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"server/internal/auth/domain"
)

type memoryUserRepo struct {
	users map[string]domain.User
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{users: make(map[string]domain.User)}
}

func (r *memoryUserRepo) FindByUsername(_ context.Context, username string) (domain.User, error) {
	u, ok := r.users[username]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *memoryUserRepo) CreateAccountWithInitialProfile(_ context.Context, username, passwordHash string) (domain.User, error) {
	u := domain.User{
		ID:           "00000000-0000-0000-0000-000000000101",
		Username:     username,
		DisplayName:  username,
		PasswordHash: passwordHash,
	}
	r.users[username] = u
	return u, nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}

func (fakeHasher) Compare(passwordHash, plainPassword string) error {
	if passwordHash != "hashed:"+plainPassword {
		return domain.ErrInvalidCredentials
	}
	return nil
}

type fakeTokenManager struct{}

func (fakeTokenManager) GenerateToken(userID, username string) (string, error) {
	return "jwt-for-" + userID + "-" + username, nil
}

func (fakeTokenManager) ValidateToken(tokenString string) (string, error) {
	return tokenString, nil
}

func TestLoginUseCaseProvisionsNewAccountAndAuthenticatesExisting(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMemoryUserRepo()
	uc := NewLoginUseCase(logger, repo, fakeHasher{}, fakeTokenManager{})

	// 1. Primer login: aprovisiona la cuenta y devuelve JWT.
	session, err := uc.Execute(context.Background(), "trace-1", "chupete_player", "secret123")
	if err != nil {
		t.Fatalf("expected first login to provision account, got error: %v", err)
	}
	if session.PlayerID == "" || session.DisplayName != "chupete_player" || session.AccessToken == "" {
		t.Fatalf("unexpected session after provisioning: %+v", session)
	}

	// 2. Segundo login con contraseña correcta: autentica la cuenta existente.
	session2, err := uc.Execute(context.Background(), "trace-2", "chupete_player", "secret123")
	if err != nil {
		t.Fatalf("expected second login with right password to succeed, got: %v", err)
	}
	if session2.PlayerID != session.PlayerID {
		t.Fatalf("expected same PlayerID %q, got %q", session.PlayerID, session2.PlayerID)
	}

	// 3. Login con contraseña incorrecta: rechaza con ErrInvalidCredentials.
	if _, err := uc.Execute(context.Background(), "trace-3", "chupete_player", "wrongpass"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLoginUseCaseValidatesUsernameAndPasswordBounds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := NewLoginUseCase(logger, newMemoryUserRepo(), fakeHasher{}, fakeTokenManager{})

	if _, err := uc.Execute(context.Background(), "trace-short-u", "ab", "secret123"); !errors.Is(err, domain.ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername for short username, got: %v", err)
	}

	if _, err := uc.Execute(context.Background(), "trace-short-p", "valid_user", "12345"); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for short password, got: %v", err)
	}
}
