package domain

import "errors"

// Errores tipados del dominio de Autenticación (auth).
// Siguiendo las reglas de DDD y Clean Architecture, estos errores son Go puro y no
// conocen códigos de estado HTTP (400/401) ni detalles de base de datos.
var (
	// ErrInvalidUsername se devuelve cuando el username no cumple el largo requerido (3 a 32 caracteres).
	ErrInvalidUsername = errors.New("el nombre de usuario debe tener entre 3 y 32 caracteres")
	// ErrInvalidPassword se devuelve cuando la contraseña no cumple el largo requerido (6 a 72 caracteres, límite de bcrypt).
	ErrInvalidPassword = errors.New("la contraseña debe tener entre 6 y 72 caracteres")
	// ErrInvalidCredentials se devuelve cuando un usuario existente ingresa una contraseña incorrecta.
	ErrInvalidCredentials = errors.New("credenciales de acceso inválidas")
	// ErrInvalidToken se devuelve cuando el token JWT está ausente, malformado, expirado o con firma inválida.
	ErrInvalidToken = errors.New("el token de acceso es inválido o ha expirado")
	// ErrUserNotFound se devuelve cuando se busca una cuenta que aún no está registrada.
	ErrUserNotFound = errors.New("el usuario solicitado no existe")
)
