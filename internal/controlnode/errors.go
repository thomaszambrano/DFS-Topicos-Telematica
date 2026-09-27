package controlnode

import (
	"errors"
	"net/http"
)

// Catálogo uniforme de errores del plano de control (RI-05).
//
// Cada uno se traduce a un código HTTP y declara si es reintentable. La distinción
// importa en el cliente: un 409 de conflicto NUNCA se reintenta automáticamente
// (RC-11), mientras que un 503 sí, con retroceso exponencial.
var (
	ErrNotFound   = errors.New("no existe")
	ErrExists     = errors.New("ya existe")
	ErrNotEmpty   = errors.New("el directorio no está vacío")
	ErrNotDir     = errors.New("no es un directorio")
	ErrIsDir      = errors.New("es un directorio")
	ErrBadRequest = errors.New("petición inválida")
	ErrConflict   = errors.New("conflicto de escritura")
	ErrNoNodes    = errors.New("sin DataNodes disponibles")
	ErrIncomplete = errors.New("carga incompleta")
)

// HTTPStatus traduce un error del dominio a su código HTTP.
func HTTPStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrExists), errors.Is(err, ErrNotEmpty), errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrNotDir),
		errors.Is(err, ErrIsDir), errors.Is(err, ErrIncomplete):
		return http.StatusBadRequest
	case errors.Is(err, ErrNoNodes):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Retryable indica si el cliente puede reintentar la misma petición tal cual.
//
// Un conflicto no es reintentable por definición: el estado del servidor no va a
// cambiar solo, y reintentar produciría la misma respuesta o, peor, una escritura
// duplicada.
func Retryable(err error) bool {
	switch {
	case errors.Is(err, ErrNoNodes):
		return true
	case HTTPStatus(err) >= 500:
		return true
	default:
		return false
	}
}
