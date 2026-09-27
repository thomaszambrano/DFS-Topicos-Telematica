package controlnode

import (
	"errors"
	"testing"

	"github.com/thomaszambrano/DFS-Topicos-Telematica/internal/types"
)

func TestMkdirYList(t *testing.T) {
	ns := NewNamespace()
	if _, err := ns.Mkdir("/media", "thomas"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := ns.Mkdir("/media/videos", "thomas"); err != nil {
		t.Fatalf("Mkdir anidado: %v", err)
	}

	entradas, err := ns.List("/media")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entradas) != 1 || entradas[0].Name != "videos" {
		t.Errorf("List devolvió %d entradas, esperaba solo 'videos'", len(entradas))
	}
}

// mkdir falla si el padre no existe: no hay creación implícita de intermedios (RC-04).
func TestMkdirExigeQueElPadreExista(t *testing.T) {
	ns := NewNamespace()
	_, err := ns.Mkdir("/no/existe/todavia", "thomas")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, obtuve %v", err)
	}
}

func TestMkdirRechazaNombreOcupado(t *testing.T) {
	ns := NewNamespace()
	if _, err := ns.Mkdir("/media", "thomas"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	_, err := ns.Mkdir("/media", "thomas")
	if !errors.Is(err, ErrExists) {
		t.Fatalf("esperaba ErrExists (409), obtuve %v", err)
	}
}

// Eliminar un directorio con hijos devuelve un error distinguible de "no existe".
func TestRemoveDirectorioNoVacio(t *testing.T) {
	ns := NewNamespace()
	if _, err := ns.Mkdir("/media", "thomas"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := ns.CreateObject("/media/video.mp4", "thomas"); err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	_, err := ns.Remove("/media")
	if !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("esperaba ErrNotEmpty (409), obtuve %v", err)
	}
	if HTTPStatus(err) != 409 {
		t.Errorf("ErrNotEmpty se traduce a %d, esperaba 409", HTTPStatus(err))
	}
}

// Las rutas con ".." se normalizan y no pueden escapar de la raíz.
func TestRutasNoEscapanDeLaRaiz(t *testing.T) {
	ns := NewNamespace()
	if _, err := ns.Mkdir("/media", "thomas"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// /media/../media se normaliza a /media y debe resolver.
	if _, err := ns.Stat("/media/../media"); err != nil {
		t.Errorf("Stat de una ruta con '..' falló: %v", err)
	}
	// Subir por encima de la raíz se normaliza a la raíz, no a un error de seguridad.
	if _, err := ns.Stat("/../../.."); err != nil {
		t.Errorf("Stat por encima de la raíz debería dar la raíz: %v", err)
	}
	// Una ruta relativa se rechaza: el contrato exige rutas absolutas.
	if _, err := ns.Stat("media"); !errors.Is(err, ErrBadRequest) {
		t.Errorf("esperaba ErrBadRequest para una ruta relativa, obtuve %v", err)
	}
}

// Hasta que se publica una versión, el objeto existe y no tiene contenido. Es lo que
// ve un lector entre el OpenUpload y el Commit.
func TestObjetoSinVersionPublicada(t *testing.T) {
	ns := NewNamespace()
	inode, err := ns.CreateObject("/video.mp4", "thomas")
	if err != nil {
		t.Fatalf("CreateObject: %v", err)
	}
	if inode.Current != nil {
		t.Error("un objeto recién creado no debería tener versión vigente")
	}
	if inode.Size() != 0 {
		t.Errorf("Size() = %d, esperaba 0", inode.Size())
	}

	v := &types.Version{ID: "v1", Size: 100, SHA256: "abc"}
	if err := ns.PublishVersion("/video.mp4", v); err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}
	de_nuevo, _ := ns.Stat("/video.mp4")
	if de_nuevo.Current == nil || de_nuevo.Size() != 100 {
		t.Error("la versión no quedó publicada")
	}
}

// Renombrar no depende del tamaño del objeto: solo mueve una referencia (RC-07).
func TestRename(t *testing.T) {
	ns := NewNamespace()
	if _, err := ns.Mkdir("/a", "thomas"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := ns.Mkdir("/b", "thomas"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := ns.CreateObject("/a/x.bin", "thomas"); err != nil {
		t.Fatalf("CreateObject: %v", err)
	}

	if err := ns.Rename("/a/x.bin", "/b/y.bin"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := ns.Stat("/a/x.bin"); !errors.Is(err, ErrNotFound) {
		t.Error("el origen sigue existiendo tras el Rename")
	}
	destino, err := ns.Stat("/b/y.bin")
	if err != nil {
		t.Fatalf("el destino no existe: %v", err)
	}
	if destino.Name != "y.bin" {
		t.Errorf("Name = %q, esperaba y.bin", destino.Name)
	}
}
