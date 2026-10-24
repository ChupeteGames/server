package domain_test

import (
	"regexp"
	"testing"

	"server/internal/matchmaking/domain"
)

func TestValidateMode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    domain.GameMode
		expectError bool
	}{
		{
			name:        "modo vacío por defecto asigna 2v2",
			input:       "",
			expected:    domain.Mode2v2,
			expectError: false,
		},
		{
			name:        "modo con espacios en blanco por defecto asigna 2v2",
			input:       "   ",
			expected:    domain.Mode2v2,
			expectError: false,
		},
		{
			name:        "modo 2v2 explícito",
			input:       "2v2",
			expected:    domain.Mode2v2,
			expectError: false,
		},
		{
			name:        "modo 3v3 explícito",
			input:       "3v3",
			expected:    domain.Mode3v3,
			expectError: false,
		},
		{
			name:        "modo 5v5 explícito",
			input:       "5v5",
			expected:    domain.Mode5v5,
			expectError: false,
		},
		{
			name:        "modo inválido devuelve ErrInvalidMode",
			input:       "10v10",
			expected:    "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := domain.ValidateMode(tc.input)
			if tc.expectError {
				if err != domain.ErrInvalidMode {
					t.Fatalf("se esperaba ErrInvalidMode pero se obtuvo: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("no se esperaba error pero se obtuvo: %v", err)
			}
			if mode != tc.expected {
				t.Fatalf("se esperaba modo %q pero se obtuvo %q", tc.expected, mode)
			}
		})
	}
}

func TestPlayersRequirementCalculations(t *testing.T) {
	if got := domain.RequiredPlayersPerTeam(domain.Mode2v2); got != 2 {
		t.Fatalf("se esperaban 2 jugadores por equipo en 2v2, se obtuvo %d", got)
	}
	if got := domain.TotalPlayersRequired(domain.Mode2v2); got != 4 {
		t.Fatalf("se esperaban 4 jugadores totales en 2v2, se obtuvo %d", got)
	}

	if got := domain.RequiredPlayersPerTeam(domain.Mode3v3); got != 3 {
		t.Fatalf("se esperaban 3 jugadores por equipo en 3v3, se obtuvo %d", got)
	}
	if got := domain.TotalPlayersRequired(domain.Mode3v3); got != 6 {
		t.Fatalf("se esperaban 6 jugadores totales en 3v3, se obtuvo %d", got)
	}
}

func TestGenerateTicketTokenFormat(t *testing.T) {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	token1 := domain.GenerateTicketToken()
	token2 := domain.GenerateTicketToken()

	if token1 == token2 {
		t.Fatalf("dos tokens generados sucesivamente no deberían ser iguales: %s vs %s", token1, token2)
	}

	if !uuidRegex.MatchString(token1) {
		t.Fatalf("el token generado %q no cumple con el formato UUID v4", token1)
	}
}
