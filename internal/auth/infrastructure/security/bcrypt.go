package security

import (
	"golang.org/x/crypto/bcrypt"

	"server/internal/auth/domain"
)

// BcryptHasher es el Adaptador Secundario que implementa domain.PasswordHasher
// utilizando el algoritmo estándar bcrypt (según docs/04_base_datos.md).
type BcryptHasher struct {
	cost int
}

var _ domain.PasswordHasher = (*BcryptHasher)(nil)

// NewBcryptHasher construye el hasher de contraseñas con el costo por defecto de bcrypt.
func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{cost: bcrypt.DefaultCost}
}

// Hash genera un hash bcrypt seguro a partir de una contraseña en texto plano.
func (h *BcryptHasher) Hash(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// Compare verifica si una contraseña en texto plano coincide con un hash bcrypt almacenado.
func (h *BcryptHasher) Compare(passwordHash, plainPassword string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(plainPassword)); err != nil {
		return domain.ErrInvalidCredentials
	}
	return nil
}
