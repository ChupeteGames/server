package domain

import (
	"crypto/rand"
	"fmt"
	"time"
)

// MatchTicket es la entidad que vincula a un jugador con una partida específica y su bando asignado.
// Su ciclo de vida es temporal y se consume al ingresar al Unreal Dedicated Server.
type MatchTicket struct {
	TicketToken  string
	MatchID      string
	PlayerID     string
	AssignedTeam TeamSide
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
	CreatedAt    time.Time
}

// ValidatedTicket representa el resultado exitoso o fallido de validar un ticket contra el Unreal Dedicated Server,
// enriquecido con los cosméticos y el MMR actual del jugador consultados desde su perfil.
type ValidatedTicket struct {
	Valid                   bool
	ReasonCode              string
	PlayerID                string
	DisplayName             string
	AssignedTeam            TeamSide
	EquippedSkinCode        string
	EquippedDeathEffectCode string
	CurrentMMR              int
}

// GenerateTicketToken genera un identificador aleatorio de 128 bits formateado como UUID v4
// sin dependencias de librerías externas para preservar la pureza del dominio.
func GenerateTicketToken() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return fmt.Sprintf("fallback-ticket-%d", time.Now().UnixNano())
	}
	// Establecer bits de versión 4 y variante RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
}
