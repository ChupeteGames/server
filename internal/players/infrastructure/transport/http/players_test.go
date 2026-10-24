package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authapp "server/internal/auth/application"
	authdomain "server/internal/auth/domain"
	"server/internal/auth/infrastructure/security"
	authpostgres "server/internal/auth/infrastructure/storage/postgres"
	authhttp "server/internal/auth/infrastructure/transport/http"
	authhandlers "server/internal/auth/infrastructure/transport/http/handlers"
	"server/internal/config"
	healthapp "server/internal/health/application"
	healthpostgres "server/internal/health/infrastructure/storage/postgres"
	healthhttp "server/internal/health/infrastructure/transport/http"
	playersapp "server/internal/players/application"
	playersdomain "server/internal/players/domain"
	playerspostgres "server/internal/players/infrastructure/storage/postgres"
	playershttp "server/internal/players/infrastructure/transport/http"
)

type inMemorySystem struct {
	users    map[string]authdomain.User
	profiles map[string]playersdomain.PlayerProfile
}

func newInMemorySystem() *inMemorySystem {
	return &inMemorySystem{
		users:    make(map[string]authdomain.User),
		profiles: make(map[string]playersdomain.PlayerProfile),
	}
}

func (s *inMemorySystem) FindByUsername(_ context.Context, username string) (authdomain.User, error) {
	u, ok := s.users[username]
	if !ok {
		return authdomain.User{}, authdomain.ErrUserNotFound
	}
	return u, nil
}

func (s *inMemorySystem) CreateAccountWithInitialProfile(_ context.Context, username, passwordHash string) (authdomain.User, error) {
	id := "11111111-2222-3333-4444-555555555555"
	u := authdomain.User{
		ID:           id,
		Username:     username,
		DisplayName:  username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	s.users[username] = u
	s.profiles[id] = playersdomain.PlayerProfile{
		PlayerID:                id,
		DisplayName:             username,
		Level:                   1,
		XP:                      0,
		MMR:                     1000,
		WalletBalance:           0,
		EquippedSkinCode:        "bean_default",
		EquippedDeathEffectCode: "gore_classic",
		OwnedCosmetics:          []string{"bean_default", "gore_classic"},
	}
	return u, nil
}

func (s *inMemorySystem) GetProfileByID(_ context.Context, playerID string) (playersdomain.PlayerProfile, error) {
	p, ok := s.profiles[playerID]
	if !ok {
		return playersdomain.PlayerProfile{}, playersdomain.ErrPlayerNotFound
	}
	return p, nil
}

func TestHTTPFlowLoginAndGetMeEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newInMemorySystem()
	hasher := security.NewBcryptHasher()
	jwtMgr := security.NewJWTManager("test_jwt_secret_key_for_http_flow")

	router := healthhttp.NewRouter(logger, healthapp.NewHealthUseCase(logger))
	authhttp.RegisterRoutes(router, logger, authapp.NewLoginUseCase(logger, store, hasher, jwtMgr))
	playershttp.RegisterRoutes(
		router,
		logger,
		authhandlers.BearerAuthMiddleware(logger, jwtMgr),
		playersapp.NewGetMyProfileUseCase(logger, store),
	)

	// 1. Llamar a GET /v1/players/me sin token debe devolver 401 Unauthorized.
	unauthReq := httptest.NewRequest(http.MethodGet, "/v1/players/me", nil)
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without token, got %d", unauthRec.Code)
	}

	// 2. Llamar a POST /v1/auth/login crea el usuario y devuelve JWT + player_id.
	loginPayload := `{"username":"nicolas_dev","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on login, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	var loginData struct {
		AccessToken string `json:"access_token"`
		PlayerID    string `json:"player_id"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginData); err != nil {
		t.Fatalf("failed to parse login response: %v", err)
	}
	if loginData.AccessToken == "" || loginData.PlayerID == "" || loginData.DisplayName != "nicolas_dev" {
		t.Fatalf("unexpected login response payload: %+v", loginData)
	}

	// 3. Llamar a GET /v1/players/me con el Bearer token devuelve el perfil con MMR 1000 y billetera 0.
	meReq := httptest.NewRequest(http.MethodGet, "/v1/players/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginData.AccessToken)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /v1/players/me, got %d: %s", meRec.Code, meRec.Body.String())
	}

	var profileData struct {
		PlayerID                string   `json:"player_id"`
		DisplayName             string   `json:"display_name"`
		Level                   int      `json:"level"`
		XP                      int64    `json:"xp"`
		MMR                     int      `json:"mmr"`
		WalletBalance           int64    `json:"wallet_balance"`
		EquippedSkinCode        string   `json:"equipped_skin_code"`
		EquippedDeathEffectCode string   `json:"equipped_death_effect_code"`
		OwnedCosmetics          []string `json:"owned_cosmetics"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &profileData); err != nil {
		t.Fatalf("failed to parse profile response: %v", err)
	}
	if profileData.MMR != 1000 || profileData.Level != 1 || profileData.WalletBalance != 0 {
		t.Fatalf("expected initial profile MMR=1000, Level=1, Wallet=0; got %+v", profileData)
	}
	if profileData.EquippedSkinCode != "bean_default" || profileData.EquippedDeathEffectCode != "gore_classic" {
		t.Fatalf("expected default equipped cosmetics, got skin=%q death_effect=%q", profileData.EquippedSkinCode, profileData.EquippedDeathEffectCode)
	}
}

func TestPostgresIntegrationAuthAndPlayers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := healthpostgres.NewPool(ctx, config.DefaultDatabaseURL)
	if err != nil {
		t.Skipf("saltando test de integración Postgres para auth/players (BD local no disponible): %v", err)
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := authpostgres.NewUserRepository(pool)
	playerRepo := playerspostgres.NewPlayerRepository(pool)
	hasher := security.NewBcryptHasher()
	jwtMgr := security.NewJWTManager(config.DefaultJWTSecret)

	loginUC := authapp.NewLoginUseCase(logger, userRepo, hasher, jwtMgr)
	getProfileUC := playersapp.NewGetMyProfileUseCase(logger, playerRepo)

	uniqueUsername := fmt.Sprintf("it_user_%d", time.Now().UnixNano()%1000000000)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE username = $1", uniqueUsername)
	}()

	session, err := loginUC.Execute(ctx, "trace-pg-1", uniqueUsername, "integrationPass123")
	if err != nil {
		t.Fatalf("Postgres LoginUseCase failed to provision account: %v", err)
	}

	profile, err := getProfileUC.Execute(ctx, "trace-pg-2", session.PlayerID)
	if err != nil {
		t.Fatalf("Postgres GetMyProfileUseCase failed: %v", err)
	}
	if profile.MMR != 1000 || profile.WalletBalance != 0 || profile.EquippedSkinCode != "bean_default" {
		t.Fatalf("unexpected initial profile in PostgreSQL: %+v", profile)
	}
}
