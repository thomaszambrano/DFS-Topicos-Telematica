package controlnode

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Auth gestiona usuarios, contraseñas y tokens de sesión (RQ-15, RQ-16).
//
// LIMITACIÓN DECLARADA: los usuarios viven en memoria, igual que el namespace; se
// pierden al reiniciar el ControlNode hasta que exista el WAL (RN-04).
type Auth struct {
	secret []byte
	ttl    time.Duration

	mu    sync.RWMutex
	users map[string][]byte // usuario -> salt (16 bytes) + hash Argon2id (32 bytes)
}

// NewAuth construye el autenticador. `secret` firma los tokens: quien la conozca
// puede emitir tokens válidos, así que nunca sale del ControlNode.
func NewAuth(secret []byte, ttl time.Duration) *Auth {
	return &Auth{secret: secret, ttl: ttl, users: make(map[string][]byte)}
}

// El nombre se usa en la ruta /home/<usuario>: se restringe para que no pueda
// contener "/" ni "..".
var validUser = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// argon2id es lento y usa 64 MB de memoria A PROPÓSITO: hace caro probar millones de
// contraseñas si alguien roba los hashes. El salt aleatorio por usuario impide que dos
// contraseñas iguales tengan el mismo hash y anula las tablas precalculadas.
func argon2id(pw string, salt []byte) []byte {
	return argon2.IDKey([]byte(pw), salt, 1, 64*1024, 4, 32)
}

// Register crea un usuario. Falla con ErrExists si el nombre ya está tomado.
func (a *Auth) Register(user, pw string) error {
	if !validUser.MatchString(user) {
		return fmt.Errorf("%w: usuario inválido (a-z, 0-9, _ y -, hasta 32)", ErrBadRequest)
	}
	if len(pw) < 8 {
		return fmt.Errorf("%w: la contraseña debe tener al menos 8 caracteres", ErrBadRequest)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	// El hash se calcula fuera del candado: tarda decenas de ms y bloquearía a todos.
	hash := argon2id(pw, salt)

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.users[user]; ok {
		return fmt.Errorf("%w: el usuario %s", ErrExists, user)
	}
	a.users[user] = append(salt, hash...)
	return nil
}

// Login devuelve un token si la contraseña es correcta. Responde el MISMO error si el
// usuario no existe o la contraseña es mala, para no revelar qué usuarios existen.
func (a *Auth) Login(user, pw string) (string, error) {
	a.mu.RLock()
	stored, ok := a.users[user]
	a.mu.RUnlock()
	if !ok || subtle.ConstantTimeCompare(argon2id(pw, stored[:16]), stored[16:]) != 1 {
		return "", fmt.Errorf("%w: usuario o contraseña incorrectos", ErrUnauthorized)
	}
	return a.sign(user), nil
}

// Token: base64(usuario|expira) + "." + HMAC-SHA256. Se verifica solo con la clave,
// sin consultar ninguna tabla (RQ-03): cambiar un byte del contenido invalida la firma.
func (a *Auth) sign(user string) string {
	exp := strconv.FormatInt(time.Now().Add(a.ttl).Unix(), 10)
	payload := base64.RawURLEncoding.EncodeToString([]byte(user + "|" + exp))
	return payload + "." + a.mac(payload)
}

func (a *Auth) mac(payload string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Verify comprueba firma y vigencia del token y devuelve el usuario que contiene.
func (a *Auth) Verify(token string) (string, error) {
	payload, sig, ok := strings.Cut(token, ".")
	// hmac.Equal compara en tiempo constante: no filtra cuántos bytes acertó un atacante.
	if !ok || !hmac.Equal([]byte(sig), []byte(a.mac(payload))) {
		return "", fmt.Errorf("%w: token inválido", ErrUnauthorized)
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("%w: token inválido", ErrUnauthorized)
	}
	user, expStr, _ := strings.Cut(string(raw), "|")
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", fmt.Errorf("%w: token vencido, vuelve a iniciar sesión", ErrUnauthorized)
	}
	return user, nil
}

type userKey struct{}

// public: salud, diagnóstico de solo lectura y las rutas de autenticación no exigen
// token. Cambiar R (PUT /v1/config/replication) sí lo exige.
func public(r *http.Request) bool {
	return r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/v1/auth/") ||
		(r.Method == http.MethodGet && (r.URL.Path == "/v1/nodes" || r.URL.Path == "/v1/df"))
}

// Middleware exige un token válido y deja el usuario en el contexto de la petición.
// Sin Auth configurado (pruebas unitarias) deja pasar todo.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public(r) {
			next.ServeHTTP(w, r)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			fail(w, fmt.Errorf("%w: falta la cabecera Authorization: Bearer <token>", ErrUnauthorized))
			return
		}
		user, err := a.Verify(token)
		if err != nil {
			fail(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

func homeOf(user string) string { return "/home/" + user }

// canAccess: cada usuario solo opera dentro de su /home (RX-13). path.Clean resuelve
// los ".." ANTES de comparar, así "/home/yo/../otro" no se cuela.
func canAccess(user, p string) bool {
	p, h := path.Clean(p), homeOf(user)
	return p == h || strings.HasPrefix(p, h+"/")
}

// onPathToHome: "/" y "/home" se pueden recorrer para llegar al home propio, pero su
// listado se filtra para no mostrar los homes ajenos.
func onPathToHome(p string) bool {
	p = path.Clean(p)
	return p == "/" || p == "/home"
}
