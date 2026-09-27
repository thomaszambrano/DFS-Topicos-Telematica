// Package types holds the core data structures for the DFSha system.
package types

import "time"

type (
	ChunkID   string
	ObjectID  string
	VersionID string
	NodeID    string
)

const DefaultChunkSize = 64 * 1024 * 1024 // Valor por omision. configurable

// ChunkMeta describe los metadatos de un chunk.
type ChunkMeta struct {
	ID       ChunkID  `json:"chunk_id"`
	Index    uint32   `json:"index"`
	Size     int64    `json:"size"`
	Checksum string   `json:"checksum"` // Valor en hexadecimal del hash SHA-256 del chunk.
	Replicas []NodeID `json:"-"`        // Estado blando, no es persistente. replicas es un cache, donde se puede reconstruir preguntando
}

// Version representa una version inmutable de un objeto.
type Version struct {
	ID        VersionID `json:"version_id"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	// cada que se itera un mapa en go no se garantiza orden, por eso usamos un slice
	Chunks []ChunkMeta `json:"chunks"`
}

// InodeKind distingue un directorio de un objeto. Es un entero con constantes en
// lugar de una cadena para que el compilador rechace valores inventados.
type InodeKind uint8

const (
	// KindDir es un directorio: tiene hijos y no tiene versiones.
	KindDir InodeKind = iota
	// KindObject es un objeto: tiene versiones y no tiene hijos.
	KindObject
)

func (k InodeKind) String() string {
	switch k {
	case KindDir:
		return "dir"
	case KindObject:
		return "object"
	default:
		return "unknown"
	}
}

// MarshalJSON emite "dir" u "object" en lugar de 0 o 1, para que el contrato REST
// sea legible y no dependa del valor numérico de la constante.
func (k InodeKind) MarshalJSON() ([]byte, error) {
	return []byte(`"` + k.String() + `"`), nil
}

// Inode es una entrada del namespace: un directorio o un objeto. Los directorios
// son entidades de primera clase con dueño y permisos propios (RN-01).
type Inode struct {
	Name       string    `json:"name"`
	Kind       InodeKind `json:"kind"`
	Owner      string    `json:"owner"`
	Mode       uint32    `json:"mode"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`

	// Children solo tiene sentido en un directorio. Es un mapa y no un slice
	// porque el patrón de acceso es buscar un hijo por nombre —lo que hace
	// resolve() en cada segmento de la ruta— y porque el orden de los hijos no
	// significa nada: ls los ordena al presentarlos. Es el caso contrario a
	// Version.Chunks, donde el orden ES el archivo.
	Children map[string]*Inode `json:"-"`

	// Current es la versión vigente si el inode es un objeto. En nil significa
	// que el objeto existe en el namespace pero todavía no tiene ninguna versión
	// publicada: es lo que ve un lector entre el OpenUpload y el Commit (RT-07).
	Current *Version `json:"-"`
}

// IsDir indica si el inode es un directorio.
func (i *Inode) IsDir() bool { return i.Kind == KindDir }

// Size devuelve el tamaño del objeto según su versión vigente. Un directorio y un
// objeto sin versión publicada miden 0.
func (i *Inode) Size() int64 {
	if i.Current == nil {
		return 0
	}
	return i.Current.Size
}
