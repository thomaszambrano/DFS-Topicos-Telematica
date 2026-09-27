package datanode

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
)

// Checksum devuelve el SHA-256 de data en hexadecimal.
//
// Hexadecimal y no base64 ni bytes crudos porque es el formato que producen
// sha256sum en la terminal, hashlib.hexdigest() en el cliente Python y este
// paquete en Go: los tres extremos del sistema comparan la misma cadena sin
// ninguna conversión de por medio (ver D-003 en DECISIONES.md).
func Checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ChecksumReader consume r por completo y devuelve su SHA-256 en hexadecimal y el
// número de bytes leídos. No carga el contenido en memoria: va alimentando el
// hash a medida que lee (RI-04).
func ChecksumReader(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// validID limita los identificadores de chunk a caracteres seguros para un nombre
// de archivo.
var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// checkID rechaza identificadores que puedan escapar del directorio de datos.
//
// El chunk_id llega por la URL y se usa para construir una ruta en disco. Sin esta
// validación, un id como "../../etc/passwd" convertiría al DataNode en un lector
// arbitrario del sistema de archivos del contenedor.
func checkID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("%w: chunk_id inválido %q", ErrBadRequest, id)
	}
	return nil
}
