# HITO2.md — Plan de implementación paso a paso

> Alcance de esta entrega: **arquitectura distribuida funcionando + especificación de
> comunicaciones**. Sin replicación, sin seguridad, sin consenso. Eso es Hito 3.
>
> Meta verificable del hito: un archivo entra, se parte en chunks, los chunks quedan
> repartidos entre DataNodes distintos, y el archivo sale idéntico.

---

## Paso 0 — Preparación del entorno

- [ ] **0.1** Instalar Go 1.22 o superior. Verificar con `go version`.
- [ ] **0.2** Crear el repositorio con esta estructura:

```
dfsha/
├── cmd/
│   ├── controlnode/main.go
│   └── datanode/main.go
├── internal/
│   ├── types/
│   ├── datanode/
│   └── controlnode/
├── api/
│   ├── openapi.yaml
│   └── dfsha.proto
├── client/            # Python
│   └── dfsha/
├── docker/
├── DECISIONES.md
└── go.mod
```

- [ ] **0.3** `go mod init github.com/<usuario>/dfsha`
- [ ] **0.4** Crear `DECISIONES.md` vacío con el formato de bitácora.

**Nota de Go:** `cmd/` contiene los ejecutables (cada carpeta con su `main.go`),
`internal/` el código que no es importable desde fuera del módulo. Es la convención
estándar y conviene respetarla.

---

# BLOQUE A — Tipos y contrato

Aquí no hay lógica. Se decide la forma de los datos, que es lo caro de cambiar después.

## A.1 — Tipos del dominio

**Archivo:** `internal/types/types.go`

- [ ] **A.1.1** Definir los identificadores como tipos propios, no como `string` pelado:

```go
type ChunkID   string
type ObjectID  string
type VersionID string
type NodeID     string
```

*Por qué:* si todos fueran `string`, el compilador dejaría pasar un `ObjectID` donde
se espera un `ChunkID`. Con tipos propios, ese error no compila.

- [ ] **A.1.2** Definir `ChunkMeta`, `Version`, `Inode`.
- [ ] **A.1.3** Definir la constante `DefaultChunkSize = 64 * 1024 * 1024`.

**Pregunta de diseño:** en `Version`, ¿por qué `Chunks` es una lista ordenada y no
un mapa? ¿Qué se rompe si se altera el orden?

---

## A.2 — Contrato de la API del cliente

**Archivo:** `api/openapi.yaml`

Solo las seis operaciones del camino feliz. El resto se especifica cuando se implemente.

- [ ] **A.2.1** `POST /v1/obj/open` → entrada: ruta, tamaño total. Salida: `upload_id`, `version_id`, `chunk_size`.
- [ ] **A.2.2** `POST /v1/obj/{upload_id}/chunk` → entrada: índice, tamaño. Salida: `chunk_id`, lista de endpoints destino.
- [ ] **A.2.3** `POST /v1/obj/{upload_id}/commit` → entrada: sha256 total. Salida: confirmación.
- [ ] **A.2.4** `GET /v1/obj/manifest?path=` → salida: lista ordenada de `{chunk_id, index, size, endpoint}` + sha256.
- [ ] **A.2.5** `GET /v1/ns?path=` y `POST /v1/ns/dir` para el namespace.
- [ ] **A.2.6** Catálogo de errores: `400`, `404` ruta inexistente, `409` clave ocupada, `503` sin DataNodes disponibles.

**Criterio de aceptación del bloque:** alguien que no conozca el código puede
implementar un cliente alterno leyendo solo este archivo. Si falta un campo para
lograrlo, el contrato está incompleto.

---

## A.3 — Contrato RPC interno

**Archivo:** `api/dfsha.proto`

- [ ] **A.3.1** `service DataNodeService` con `Heartbeat(NodeReport) returns (CommandList)`.
- [ ] **A.3.2** Mensajes `NodeReport` (id, capacidad, usado, lista de chunks) y `Command`.
- [ ] **A.3.3** Generar el código: `protoc --go_out=. --go-grpc_out=. api/dfsha.proto`

**Nota:** en este hito el heartbeat solo sirve para que el ControlNode sepa qué
DataNodes existen y cuánto espacio tienen. Las órdenes vienen en Hito 3, pero el
campo se deja declarado desde ahora.

---

# BLOQUE B — DataNode (Go)

El componente más simple y el mejor para aprender Go. No conoce el namespace:
solo guarda y sirve bytes identificados por `chunk_id`.

## B.1 — Almacén de chunks

**Archivo:** `internal/datanode/store.go`

- [ ] **B.1.1** Struct `Store` con el directorio base y el tamaño total usado.

```go
type Store struct {
    baseDir string
    mu      sync.RWMutex
    used    int64
}
```

- [ ] **B.1.2** `func NewStore(baseDir string) (*Store, error)` — crea el directorio si no existe y calcula el espacio usado recorriéndolo.

- [ ] **B.1.3** `func (s *Store) Put(id ChunkID, data []byte) error`

**Pregunta de diseño que decide la implementación:** ¿qué pasa si el proceso muere
a mitad de la escritura? Un archivo a medias con nombre válido es peor que ningún
archivo, porque el sistema lo creerá bueno.

*Pista:* la técnica se llama **escritura atómica por renombrado**: escribes en
`<id>.tmp`, haces `f.Sync()`, y solo entonces `os.Rename()` al nombre definitivo.
El renombrado dentro del mismo sistema de archivos es atómico, así que el archivo
final o existe completo o no existe.

- [ ] **B.1.4** `func (s *Store) Get(id ChunkID) ([]byte, error)` — lee y **verifica el checksum antes de devolver**.

- [ ] **B.1.5** `func (s *Store) Has(id ChunkID) bool`

- [ ] **B.1.6** `func (s *Store) Inventory() ([]ChunkID, error)` — lista los chunks presentes en disco.

- [ ] **B.1.7** `func (s *Store) Stats() (used, capacity int64)`

**Comentario obligatorio:** en `Put`, explicar *por qué* se escribe a temporal y se
renombra. Si no puedes escribir ese comentario, no entiendes la función todavía.

---

## B.2 — Integridad

**Archivo:** `internal/datanode/checksum.go`

- [ ] **B.2.1** `func Checksum(data []byte) []byte` — SHA-256 del contenido.
- [ ] **B.2.2** Decidir dónde se guarda el checksum: ¿archivo aparte `<id>.sum`, o recalculado en cada lectura?

**Pregunta:** recalcular en cada lectura es más simple pero cuesta CPU en cada `get`.
Guardarlo aparte es más rápido pero añade un archivo que puede desincronizarse.
Elige y anótalo en `DECISIONES.md`.

---

## B.3 — Servidor HTTP del DataNode

**Archivo:** `internal/datanode/server.go`

- [ ] **B.3.1** `func (d *DataNode) handlePutChunk(w http.ResponseWriter, r *http.Request)`

**Importante:** leer el cuerpo con `io.Copy` hacia el archivo, **no** con
`io.ReadAll`. Con chunks de 64 MB y varias subidas concurrentes, `ReadAll` te come
la memoria. Este es el requisito RI-04.

- [ ] **B.3.2** `func (d *DataNode) handleGetChunk(...)` — sirve el chunk por flujo.
- [ ] **B.3.3** `func (d *DataNode) handleHealth(...)` — devuelve espacio libre, usado y número de chunks.
- [ ] **B.3.4** Registrar rutas: `PUT /chunk/{id}`, `GET /chunk/{id}`, `GET /health`.

---

## B.4 — Agente de heartbeat

**Archivo:** `internal/datanode/heartbeat.go`

- [ ] **B.4.1** `func (d *DataNode) startHeartbeat(ctx context.Context, interval time.Duration)`

Lanza una goroutine con un `time.Ticker` que cada N segundos envía el `NodeReport`
al ControlNode.

**Punto de diseño que hay que respetar desde ya:** el DataNode **siempre inicia** la
conexión. El ControlNode nunca abre una conexión hacia él. Aunque en este hito las
órdenes estén vacías, la dirección del control queda fijada.

- [ ] **B.4.2** Incluir en el reporte el `advertised_endpoint` configurable, distinto de la dirección interna (requisito RD-12).

---

## B.5 — Ejecutable

**Archivo:** `cmd/datanode/main.go`

- [ ] **B.5.1** Leer configuración de variables de entorno: `NODE_ID`, `DATA_DIR`, `LISTEN_ADDR`, `ADVERTISED_ENDPOINT`, `CONTROL_ADDR`, `CAPACITY`.
- [ ] **B.5.2** Construir `Store`, arrancar heartbeat y servidor HTTP.
- [ ] **B.5.3** Apagado ordenado con `signal.NotifyContext`.

**Criterio de aceptación del bloque B:** levantas un DataNode, le haces `PUT` de un
archivo con `curl`, lo recuperas con `GET`, y los `sha256sum` coinciden. Todavía sin
ControlNode.

---

# BLOQUE C — ControlNode (Go)

## C.1 — Registro de DataNodes

**Archivo:** `internal/controlnode/registry.go`

- [ ] **C.1.1** Struct `Registry` con un mapa `NodeID → NodeState` protegido por mutex.
- [ ] **C.1.2** `func (r *Registry) Update(report *NodeReport)` — registra o actualiza el nodo con su último heartbeat.
- [ ] **C.1.3** `func (r *Registry) Alive() []*NodeState` — nodos con heartbeat reciente.

**Nota de Go:** un mapa no es seguro para acceso concurrente. Como el heartbeat
escribe y las peticiones leen a la vez, necesitas `sync.RWMutex`: `RLock()` para
leer, `Lock()` para escribir.

---

## C.2 — Namespace

**Archivo:** `internal/controlnode/namespace.go`

- [ ] **C.2.1** Struct `Namespace` con el árbol de inodes en memoria.
- [ ] **C.2.2** `func (ns *Namespace) resolve(path string) (*Inode, error)` — recorre el árbol segmento a segmento. **Escribe esta primero**: todas las demás la usan.
- [ ] **C.2.3** `func (ns *Namespace) Mkdir(path string) error`
- [ ] **C.2.4** `func (ns *Namespace) List(path string) ([]*Inode, error)`
- [ ] **C.2.5** `func (ns *Namespace) Stat(path string) (*Inode, error)`
- [ ] **C.2.6** `func (ns *Namespace) CreateObject(path string) (*Inode, error)`
- [ ] **C.2.7** `func (ns *Namespace) Remove(path string) error`

**Pregunta de diseño:** `rmdir` sobre un directorio con hijos debe fallar. ¿Dónde
haces esa comprobación, y qué error devuelves para que el cliente lo distinga de
"no existe"?

---

## C.3 — Colocación (versión simple del hito)

**Archivo:** `internal/controlnode/placement.go`

- [ ] **C.3.1** `func (p *Placer) Select(index uint32, r int) ([]*NodeState, error)`

En este hito `r = 1`: sin replicación. Pero **el reparto entre nodos ya debe
funcionar**, porque es el requisito central del enunciado (RX-11, RX-12).

- [ ] **C.3.2** Implementar la regla mínima: chunks consecutivos van a nodos distintos.

**Deliberadamente simple por ahora.** La heurística de dos candidatos con puntuación
es del Hito 3. Pero deja la firma preparada para recibir `r > 1`.

---

## C.4 — Gestor de cargas

**Archivo:** `internal/controlnode/allocator.go`

- [ ] **C.4.1** Struct `Upload` con `upload_id`, ruta, versión en construcción, chunks asignados.
- [ ] **C.4.2** `func (a *Allocator) OpenUpload(path string, size uint64) (*Upload, error)` — valida que la ruta sea válida y el padre exista.
- [ ] **C.4.3** `func (a *Allocator) AllocateChunk(uploadID string, index uint32) (*ChunkPlan, error)` — genera `chunk_id` y elige destino.
- [ ] **C.4.4** `func (a *Allocator) Commit(uploadID string, sha []byte) error` — **publica la versión; es la última operación y debe ser atómica**.
- [ ] **C.4.5** `func (a *Allocator) Manifest(path string) (*Manifest, error)` — devuelve la lista ordenada de chunks con sus endpoints.

**Pregunta central:** hasta el `Commit`, ¿qué ve un cliente que hace `get` de esa
misma ruta? La respuesta define todo el modelo de consistencia.

---

## C.5 — Servidor HTTP del ControlNode

**Archivo:** `internal/controlnode/server.go`

- [ ] **C.5.1** Handlers de las seis operaciones de `openapi.yaml`.
- [ ] **C.5.2** Servidor gRPC para recibir heartbeats.
- [ ] **C.5.3** Middleware de logging que imprima cada asignación de chunk con su destino (requisito RC-08, el profesor lo pidió explícitamente).

---

## C.6 — Ejecutable

**Archivo:** `cmd/controlnode/main.go`

- [ ] **C.6.1** Configuración por variables de entorno.
- [ ] **C.6.2** Arrancar HTTP y gRPC en puertos distintos, cada uno en su goroutine.

**Criterio de aceptación del bloque C:** con `curl` puedes hacer `mkdir`, `ls`, abrir
una carga, pedir asignación de tres chunks y ver que el ControlNode devuelve
DataNodes **distintos** para cada uno.

---

# BLOQUE D — Cliente (Python)

## D.1 — Particionamiento

**Archivo:** `client/dfsha/chunker.py`

- [ ] **D.1.1** `def split_file(path, chunk_size) -> Iterator[tuple[int, bytes]]`

Genera `(índice, datos)`. **El último chunk es más corto y no se rellena.**

Esta es la función que el profesor mencionó con la historia del `.exe` mal
particionado. Escríbela con cuidado y coméntala bien.

- [ ] **D.1.2** `def sha256_of_file(path) -> str`
- [ ] **D.1.3** `def sha256_of_bytes(data) -> str`

**Prueba inmediata:** partir un archivo, reensamblar los chunks en orden, comparar
hashes. Antes de tocar la red.

---

## D.2 — Cliente HTTP

**Archivo:** `client/dfsha/api.py`

- [ ] **D.2.1** Clase `ControlClient` con un método por operación del contrato.
- [ ] **D.2.2** Clase `DataClient` con `put_chunk` y `get_chunk`.
- [ ] **D.2.3** Manejo de errores que distinga reintentable de no reintentable.

---

## D.3 — Transferencia

**Archivo:** `client/dfsha/transfer.py`

- [ ] **D.3.1** `def put(local_path, remote_path)` — `open` → bucle de `allocate` + subida → `commit`.
- [ ] **D.3.2** Log por cada chunk: índice, tamaño, DataNode destino, tiempo.
- [ ] **D.3.3** `def get(remote_path, local_path)` — `manifest` → **descargas en paralelo** → reensamblar en orden → verificar hash.

Para el paralelismo: `ThreadPoolExecutor`. Descarga concurrente, escritura ordenada.

**Pregunta:** si descargas en paralelo pero escribes en orden, ¿dónde guardas los
chunks que llegan antes de que les toque? ¿Qué pasa con la memoria si el archivo
tiene 240 chunks de 64 MB?

---

## D.4 — Shell

**Archivo:** `client/dfsha/shell.py`

- [ ] **D.4.1** REPL con `ls`, `cd`, `mkdir`, `rmdir`, `rm`, `stat`, `put`, `get`, `exit`.
- [ ] **D.4.2** El directorio de trabajo es **estado del cliente**, no del servidor.
- [ ] **D.4.3** Resolución de rutas relativas, absolutas, `.` y `..`.

---

# BLOQUE E — Despliegue y demostración

## E.1 — Contenedores

- [ ] **E.1.1** `docker/Dockerfile.controlnode` y `docker/Dockerfile.datanode` con build multi-etapa.
- [ ] **E.1.2** `docker-compose.yml` con 1 ControlNode y **4 DataNodes**, cada uno con su volumen.

## E.2 — Verificación del hito

- [ ] **E.2.1** Levantar el clúster, crear directorios, listar.
- [ ] **E.2.2** `put` de un archivo grande; capturar el log que muestra el reparto.
- [ ] **E.2.3** **Entrar a los volúmenes de los 4 DataNodes y comprobar que cada uno tiene chunks distintos.** Esta es la evidencia de RX-12.
- [ ] **E.2.4** `get` y comparar `sha256sum` con el original.
- [ ] **E.2.5** Registrar todo en `DECISIONES.md`.

---

# Orden de trabajo sugerido

| Día | Bloque |
| --- | --- |
| 1 | A completo (tipos y contrato) |
| 2–3 | B.1 a B.3 (almacén y servidor del DataNode) |
| 4 | B.4, B.5 y C.1 (heartbeat y registro) |
| 5–6 | C.2 a C.4 (namespace y asignación) |
| 7 | C.5, C.6 |
| 8–9 | D completo (cliente) |
| 10 | E (docker y verificación) |

Si el tiempo se acorta: el namespace puede quedar solo en memoria, sin WAL, y se
declara como limitación conocida. Lo que **no** se puede sacrificar es el reparto
real de chunks entre nodos y la verificación de integridad.

---

# Preguntas que debes poder responder al entregar

1. ¿Por qué el último chunk no se rellena?
2. ¿Por qué `Put` escribe a temporal y renombra?
3. ¿Por qué el DataNode inicia el heartbeat y no al revés?
4. ¿Por qué los datos no pasan por el ControlNode?
5. ¿Qué ve un lector antes del `commit`?
6. ¿Por qué el DataNode no conoce el namespace?
7. ¿Por qué `io.Copy` y no `io.ReadAll`?
8. ¿Cómo garantizas que dos chunks consecutivos no caigan en el mismo nodo?
