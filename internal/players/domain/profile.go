package domain

import "context"

// CareerStats agrupa las estadísticas históricas acumuladas por el jugador a lo largo de su carrera.
type CareerStats struct {
	MatchesPlayed int
	MatchesWon    int
	MatchesDrawn  int
	TotalKills    int
	TotalDeaths   int
	MutualKills   int
	DuelsWon      int
}

// PlayerProfile es el Agregado de Dominio del contexto de Jugadores (players).
// Consolida la progresión (nivel y XP), el puntaje competitivo (MMR base 1000),
// el saldo de su billetera, los cosméticos que lleva equipados y su inventario desbloqueado.
type PlayerProfile struct {
	PlayerID                string
	DisplayName             string
	Level                   int
	XP                      int64
	MMR                     int
	WalletBalance           int64
	EquippedSkinCode        string
	EquippedDeathEffectCode string
	OwnedCosmetics          []string
	CareerStats             CareerStats
}

// PlayerRepository es el Puerto de Salida (Secondary Port) para consultar y actualizar el perfil del jugador.
type PlayerRepository interface {
	GetProfileByID(ctx context.Context, playerID string) (PlayerProfile, error)
}
