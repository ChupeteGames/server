package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"server/internal/auth/domain"
)

const (
	// defaultTokenTTL define la duración estándar de una sesión de juego (24 horas).
	defaultTokenTTL = 24 * time.Hour
)

// JWTManager es el Adaptador Secundario que implementa domain.TokenManager
// emitiendo y verificando tokens JWT estándar (RFC 7519) firmados con HMAC-SHA256 (HS256)
// utilizando únicamente la librería criptográfica estándar de Go.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

var _ domain.TokenManager = (*JWTManager)(nil)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub      string `json:"sub"`
	Username string `json:"username"`
	Iat      int64  `json:"iat"`
	Exp      int64  `json:"exp"`
}

// NewJWTManager crea un gestor de tokens JWT con la clave secreta provista.
func NewJWTManager(secret string) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		ttl:    defaultTokenTTL,
		now:    time.Now,
	}
}

// GenerateToken firma un JWT HS256 conteniendo el ID del jugador (sub) y su username.
func (m *JWTManager) GenerateToken(userID, username string) (string, error) {
	headerBytes, err := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}

	now := m.now().UTC()
	claimsBytes, err := json.Marshal(jwtClaims{
		Sub:      userID,
		Username: username,
		Iat:      now.Unix(),
		Exp:      now.Add(m.ttl).Unix(),
	})
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := encodedHeader + "." + encodedClaims

	signature := m.sign(signingInput)
	return signingInput + "." + signature, nil
}

// ValidateToken verifica la estructura, la firma HMAC-SHA256 en tiempo constante y la fecha
// de expiración del token JWT, devolviendo el player_id (claim "sub") si es válido.
func (m *JWTManager) ValidateToken(tokenString string) (string, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return "", domain.ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := m.sign(signingInput)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return "", domain.ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", domain.ErrInvalidToken
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil || hdr.Alg != "HS256" {
		return "", domain.ErrInvalidToken
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", domain.ErrInvalidToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return "", domain.ErrInvalidToken
	}

	if claims.Sub == "" || m.now().UTC().Unix() >= claims.Exp {
		return "", domain.ErrInvalidToken
	}

	return claims.Sub, nil
}

func (m *JWTManager) sign(signingInput string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
