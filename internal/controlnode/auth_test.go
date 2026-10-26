package controlnode

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newSecureServer(t *testing.T) http.Handler {
	t.Helper()
	reg := NewRegistry(9*time.Second, 30*time.Second)
	ns := NewNamespace()
	alloc := NewAllocator(ns, NewPlacer(reg), reg, 4<<20, 1, 1, time.Minute)
	s := NewServer(ns, reg, alloc, 3*time.Second)
	s.AttachAuth(NewAuth([]byte("clave-de-prueba"), time.Minute))
	return s.Routes()
}

func call(h http.Handler, method, url, token string, body any) (int, map[string]any) {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func login(t *testing.T, h http.Handler, user string) string {
	t.Helper()
	creds := map[string]string{"user": user, "password": "contraseña-larga"}
	if code, _ := call(h, "POST", "/v1/auth/register", "", creds); code != http.StatusCreated {
		t.Fatalf("register %s: %d", user, code)
	}
	code, out := call(h, "POST", "/v1/auth/login", "", creds)
	if code != http.StatusOK {
		t.Fatalf("login %s: %d", user, code)
	}
	return out["token"].(string)
}

func TestSinTokenEs401(t *testing.T) {
	h := newSecureServer(t)
	if code, _ := call(h, "GET", "/v1/ns?path=/", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("sin token: %d, se esperaba 401", code)
	}
	if code, _ := call(h, "GET", "/v1/ns?path=/", "basura.firma", nil); code != http.StatusUnauthorized {
		t.Fatalf("token falso: %d, se esperaba 401", code)
	}
	if code, _ := call(h, "GET", "/health", "", nil); code != http.StatusOK {
		t.Fatalf("/health debe ser público: %d", code)
	}
}

func TestContraseñaIncorrectaEs401(t *testing.T) {
	h := newSecureServer(t)
	login(t, h, "ana")
	bad := map[string]string{"user": "ana", "password": "otra-contraseña"}
	if code, _ := call(h, "POST", "/v1/auth/login", "", bad); code != http.StatusUnauthorized {
		t.Fatalf("contraseña mala: %d, se esperaba 401", code)
	}
}

func TestAislamientoPorUsuario(t *testing.T) {
	h := newSecureServer(t)
	ana, beto := login(t, h, "ana"), login(t, h, "beto")

	if code, _ := call(h, "POST", "/v1/ns/dir", ana, map[string]string{"path": "/home/ana/fotos"}); code != http.StatusCreated {
		t.Fatalf("mkdir en el home propio: %d", code)
	}
	for _, p := range []string{"/home/ana", "/home/beto/../ana", "/otro"} {
		if code, _ := call(h, "GET", "/v1/ns?path="+p, beto, nil); code != http.StatusForbidden {
			t.Fatalf("beto listando %s: %d, se esperaba 403", p, code)
		}
	}
	_, out := call(h, "GET", "/v1/ns?path=/home", beto, nil)
	entries := out["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["name"] != "beto" {
		t.Fatalf("beto en /home debe ver solo su home, vio %v", entries)
	}
	rename := map[string]string{"from": "/home/ana/fotos", "to": "/home/beto/fotos"}
	if code, _ := call(h, "POST", "/v1/ns/rename", beto, rename); code != http.StatusForbidden {
		t.Fatalf("rename de un directorio ajeno: %d, se esperaba 403", code)
	}
}
