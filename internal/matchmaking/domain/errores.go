package domain

import "errors"

// Errores tipados del dominio de Matchmaking y Gestión de Partidas.
// Siguiendo los principios de DDD y Clean Architecture, estos errores representan invariantes
// de negocio en Go puro, sin acoplamiento a HTTP (400/404) ni códigos de estado gRPC.
var (
	// ErrInvalidMode se devuelve cuando se solicita un modo de juego no soportado.
	ErrInvalidMode = errors.New("modo de juego inválido, los modos soportados son: 2v2, 3v3, 4v4, 5v5")

	// ErrAlreadyInQueue se devuelve cuando un jugador que ya está en la cola intenta encolarse nuevamente.
	ErrAlreadyInQueue = errors.New("el jugador ya se encuentra esperando en la cola de matchmaking")

	// ErrNotInQueue se devuelve cuando se intenta cancelar una búsqueda y el jugador no estaba en cola.
	ErrNotInQueue = errors.New("el jugador no se encuentra en la cola de matchmaking")

	// ErrPlayerIDRequired se devuelve cuando el identificador de jugador viene vacío.
	ErrPlayerIDRequired = errors.New("el ID del jugador es obligatorio")

	// ErrMatchNotFound se devuelve cuando la partida solicitada no existe en el sistema.
	ErrMatchNotFound = errors.New("la partida solicitada no existe")

	// ErrTicketNotFound se devuelve cuando el ticket de ingreso no existe en el sistema.
	ErrTicketNotFound = errors.New("el ticket de acceso no existe")

	// ErrTicketExpired se devuelve cuando el ticket ha superado su tiempo de validez.
	ErrTicketExpired = errors.New("el ticket de acceso ha expirado")

	// ErrTicketAlreadyUsed se devuelve cuando el ticket ya fue consumido en un ingreso previo.
	ErrTicketAlreadyUsed = errors.New("el ticket de acceso ya fue utilizado")

	// ErrMatchMismatch se devuelve cuando el ticket no corresponde al match_id provisto por Unreal Dedicated Server.
	ErrMatchMismatch = errors.New("el ticket no corresponde a la partida indicada")

	// ErrMatchNotActive se devuelve cuando la partida ya no está en estado created o in_progress.
	ErrMatchNotActive = errors.New("la partida no se encuentra activa")
)
