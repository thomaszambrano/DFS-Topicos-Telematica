# Análisis de la capa de consistencia de DFSha

Archivos analizados: `internal/controlnode/{allocator,registry,placement,namespace}.go`, `api/dfsha.proto`, `internal/datanode/{heartbeat,store,checksum}.go`, `client/dfsha/chunker.py`.

Nota: es un análisis por lectura de código. No se ejecutó nada; los bugs marcados no están reproducidos.

## 1. Arquitectura: dos clases de estado

| | Estado duro | Estado blando |
|---|---|---|
| Qué es | `Namespace` (árbol, versiones, orden de chunks) | `Registry.locations`, `nodes`, `expected` |
| Dónde está la verdad | Solo en el ControlNode | En el disco de cada DataNode |
| Si se pierde | Los bytes quedan sin dueño, el dato está perdido | Se reconstruye con los heartbeats |
| Persistencia | WAL + fsync (pendiente del Hito 3) | Ninguna, a propósito |

`Allocator` es el único componente que toca los dos lados. Por eso concentra la consistencia: decide cuándo algo pasa de "en construcción" a "visible".

## 2. `allocator.go`

### Modelo de consistencia

1. **Escritura en dos fases.** `OpenUpload` → `AllocateChunk`×N → `Commit`. Hasta el Commit todo vive en `Upload.chunks`, que ningún lector ve. El objeto sigue apuntando a su `Current` anterior.
2. **Commit atómico.** `PublishVersion` es una asignación de puntero (`node.Current = v`) bajo el lock de escritura del namespace (`namespace.go:231`). No se copian datos, así que no hay estado intermedio observable. Es la decisión central del sistema.
3. **Exclusión por lease por ruta.** Dos escritores sobre la misma ruta no se fusionan: el segundo recibe `ErrConflict` (409). El lease vence por TTL, así que un cliente muerto no bloquea la ruta para siempre. Se renueva en cada `AllocateChunk`.
4. **Idempotencia en dos niveles.**
   - Por `idempotencyKey` en `OpenUpload`.
   - Por índice en `AllocateChunk`, que devuelve el mismo `chunk_id` si se reintenta (`allocator.go:223`).

   Un timeout del cliente no genera versiones duplicadas ni chunks huérfanos.
5. **El cliente ejecuta el plan, no lo decide.** El ControlNode genera el `chunk_id` (aleatorio y opaco, para no filtrar la ruta al plano de datos) y elige los destinos. El tráfico del plano de control crece con el número de chunks, no con su tamaño.
6. **Validaciones del Commit.**
   - Sin huecos: el índice `i` debe estar en la posición `i`.
   - `Expected == len(ordered)`.
   - La suma de tamaños coincide con `Size`.
7. **`Manifest` resuelve las direcciones al emitirse.** Los endpoints salen del `Registry`, que sale de los heartbeats. Ninguna ruta del namespace codifica un nodo (transparencia de localización).
8. **`ExpectReplicas` en el Commit.** Cubre la ventana de hasta 3 s entre el commit y el próximo heartbeat. Sin esto, leer justo después de escribir daría 503 (read-your-writes).

### Orden de locks
`Allocator.mu → Namespace.mu → Registry.mu`. Ningún componente de abajo llama hacia arriba, así que no hay deadlock posible. El lock global del Allocator serializa todas las cargas; a esta escala está bien.

### Problemas encontrados

| # | Gravedad | Dónde | Problema |
|---|---|---|---|
| A1 | Media | `Manifest`, `allocator.go:402-413` | **Data race.** `Stat` devuelve `*Inode` y suelta el RLock. Después se lee `inode.Current` sin lock, mientras `PublishVersion` lo escribe. Las `Version` son inmutables, pero la lectura del puntero compite con la escritura. Confirmar con `go test -race`. Arreglo: que `Stat` devuelva una copia, o leer `Current` dentro del lock. `List` tiene el mismo problema. |
| A2 | Media | `Commit` | **Se confía en que el cliente escribió los chunks.** El Commit no verifica nada contra los DataNodes. Si el cliente miente o falla en silencio, se publica una versión cuyos chunks no existen; las lecturas dan 503 cuando caduca `expected`. No rompe la atomicidad pero sí la durabilidad. Mitigación: exigir ACK por réplica. |
| A3 | Baja | `Commit` | Con `Size == 0` y chunks subidos, `Expected == 0` y `up.Size > 0` es falso, así que se acepta cualquier cantidad de chunks. |
| A4 | Baja | `Commit` | No se valida que todos los chunks salvo el último midan `ChunkSize`. |
| A5 | Baja | `OpenUpload` | Si el cliente aborta o vence el lease en un archivo nuevo, queda un inodo con `Current == nil`: ruta visible sin contenido. `Abort` no lo limpia. |
| A6 | Baja | `OpenUpload:153` | El hit de idempotencia devuelve el `Upload` sin comprobar `ExpiresAt`. |
| A7 | Baja | `AllocateChunk:223` | El reintento por índice ignora `size` y `checksum` nuevos y conserva los viejos sin avisar. |

## 3. `registry.go`

- **`Update` reemplaza el inventario completo.** Retira las ubicaciones antiguas del nodo y añade las nuevas. Si un nodo pierde el disco, sus chunks desaparecen en el siguiente heartbeat. `node.inventory` evita recorrer el mapa global.
- **`classify`** usa los umbrales alive/suspect/dead.
  - `Alive()` (para colocar) excluye suspect y draining.
  - `holders()` (para leer) solo excluye dead.
  - La asimetría es intencional: un nodo suspect no recibe escrituras nuevas, pero sus datos se siguen sirviendo, y un reinicio rápido no dispara re-replicación.
- **`expected` con TTL = `suspectAfter`.** Suposición optimista que caduca sola.
- **R1 (media), `registry.go:158`.** El primer heartbeat de un nodo borra su `expected`, aunque ese inventario se haya armado antes de que el chunk llegara a disco.
  - Secuencia: el DataNode arma el inventario, el cliente termina de escribir y hace Commit, llega el heartbeat viejo.
  - El `expected` se borra, el chunk no está en `locations`, y `Manifest` falla con `ErrNoNodes` hasta el siguiente heartbeat (~3 s).
  - Ventana estrecha. Arreglo: borrar solo los `expected` cuyo chunk aparece en el inventario y dejar que el TTL elimine el resto.
- **R2 (baja).** `Totals()` cuenta `len(r.locations)` (chunks distintos). Con R>1 no coincide con la suma de `n.Chunks`. Solo afecta a `df`.

## 4. `placement.go`

Algoritmo:
1. Filtra nodos vivos, no drenando, con espacio libre.
2. Rota la lista por `index % len(usable)`.
3. Toma `r` nodos consecutivos.

Cumple RN-08 (réplicas en nodos distintos) y RN-09 (chunks consecutivos arrancan en nodos distintos). Si hay menos de `r` nodos devuelve menos y marca `Degraded` en vez de rechazar.

Problemas:
- **P1 (media).** Determinista y sin estado: todos los archivos empiezan el chunk 0 en el mismo primer candidato. Los archivos pequeños (1 chunk) se apilan en el nodo menos ocupado hasta que un heartbeat actualiza `Used`. Solución: `offset` aleatorio o contador global.
- **P2 (baja).** La lista se ordena por ocupación, que cambia con cada heartbeat. La rotación por índice solo es estable si el orden no cambia entre dos `AllocateChunk`. RN-09 es blanda, así que no se viola formalmente.
- **P3 (baja).** `Free() > 0` no considera el tamaño del chunk. Un nodo con 1 KB libre puede ser elegido para 64 MB y devolver `ErrNoSpace`.
- La heurística de dos candidatos aleatorios (RN-10/11) es del Hito 3. La firma de `Select` ya admite `r > 1`.

## 5. `namespace.go`

- Un solo `RWMutex` para todo el árbol (RN-25): a esta escala la contención no pesa y los locks por nodo causan deadlocks.
- `splitPath` hace `path.Clean` y rechaza rutas no absolutas, lo que bloquea `..` y escapadas de la raíz.
- `Rename` es O(1) y no toca ningún DataNode: solo mueve una referencia.
- `Remove` devuelve los chunks huérfanos. El recolector es del Hito 3.
- **N1 (media), bug.** `Rename` no impide mover un directorio dentro de sí mismo. `Rename("/a", "/a/b/x")` con `/a/b` existente borra `/a` del padre y lo cuelga bajo su propio subárbol: ciclo inalcanzable desde la raíz y subárbol perdido. Falta comprobar que `to` no tenga a `from` como prefijo.
- **N2 (baja).** `Remove` de un objeto con carga activa no cancela el lease. El Commit posterior falla con NotFound y los chunks quedan huérfanos.
- **N3 (baja).** `CreateObject` devuelve el objeto existente si ya hay uno, lo que permite sobrescribir con versiones nuevas.
- Limitación declarada: todo en memoria; reiniciar el ControlNode borra el namespace. El WAL es del Hito 3.

## 6. `dfsha.proto` y `heartbeat.go`

- **Dirección de la conexión.** El DataNode siempre inicia; el ControlNode nunca llama. Un único RPC, `Heartbeat(NodeReport) → CommandList`, con las órdenes en la respuesta. Los DataNodes pueden estar tras NAT.
- **`NodeReport` lleva el inventario completo, no un delta.** Es lo que hace el mapa de ubicaciones estado blando. Costo: el reporte crece con el número de chunks y `Store.Inventory()` hace un `ReadDir` cada 3 s.
- `advertised_endpoint` se reporta explícitamente; la IP interna de un contenedor no sirve a los clientes.
- `grpc.NewClient` reconecta solo: un ControlNode caído no tumba al DataNode.
- El primer beat va inmediato; timeout de 5 s por llamada.
- `execute` solo registra logs; las órdenes reales llegan en el Hito 3.
- **H1 (baja).** Sin backoff ni jitter ante errores.
- **H2 (baja).** `Draining` fijo en `false`: el drenado no funciona todavía.
- **H3 (baja).** `CommandList.heartbeat_interval_ms` se ignora; el intervalo se fija al arrancar.

## 7. `store.go` y `checksum.go`

- **`Put` es atómico por renombrado:**
  1. Escribe a un `.tmp`.
  2. SHA-256 en la misma pasada (`TeeReader` + `countingWriter`), sin cargar el chunk en memoria.
  3. Compara con el checksum esperado.
  4. `Sync` del archivo.
  5. `rename` al nombre final.
  6. `syncDir`.

  El chunk existe completo o no existe. `NewStore` borra los `.tmp` huérfanos al arrancar.
- **Checksum en archivo `.sum` aparte** para no pagar un SHA-256 completo por GET. `Verify` recalcula contra el guardado y servirá de base al barrido de integridad.
- **`checkID`** (`^[A-Za-z0-9_-]{1,128}$`) impide path traversal, ya que el `chunk_id` llega por URL.
- Hexadecimal en todos lados: Go, Python y `sha256sum` comparan la misma cadena.

Problemas:
- **S1 (media).** El `.sum` se escribe con `os.WriteFile` sin `Sync`. Tras un corte de energía puede quedar un chunk completo con `.sum` vacío o ausente, que `Open` reporta como corrupto. Debería escribirse con tmp + sync + rename.
- **S2 (media).** Si se reescribe un `chunk_id` existente (reintento tras timeout), el rename sobrescribe y `used += n; count++` se aplica otra vez. Los contadores derivan hasta el reinicio.
- **S3 (baja).** `Put` comprueba `used >= capacity`, no `used + incoming > capacity`.
- **S4 (baja).** Un crash entre escribir el `.sum` y renombrar el chunk deja un `.sum` huérfano, que `NewStore` no limpia.
- **S5 (baja).** `Inventory()` lista los `.chunk` sin verificar que exista su `.sum`; un chunk sin `.sum` se anuncia y falla al leerse.

## 8. `chunker.py`

- `split_file` es un generador: subir 15 GB no requiere 15 GB de RAM.
- **El último chunk conserva su tamaño real, sin relleno** (RT-02). Mantiene igual el SHA-256 del archivo reconstruido.
- `chunk_count` usa división con techo, con la misma fórmula que `OpenUpload` en Go. Esa coincidencia es crítica porque `Commit` valida `Expected`.
- Un archivo vacío no produce chunks; cuadra con `Expected = 0` (ver A3).
- Sin problemas funcionales. Depende de que el llamador pase el `chunk_size` que informa el servidor y no el default de 64 MB.

## 9. Prioridades

**Corregir antes de la demo o la entrega:**
1. **N1**: `Rename` de un directorio dentro de sí mismo (pierde datos).
2. **A1**: data race en `Manifest` y `List` (confirmar con `go test -race`).
3. **S2**: doble conteo en `Put` con `chunk_id` repetido.
4. **S1**: `.sum` sin fsync.
5. **R1**: el heartbeat borra `expected` de forma prematura.

**Decisiones de diseño a justificar ante el profesor:**
- Por qué el commit es un puntero y no una copia.
- Por qué las ubicaciones son estado blando y el namespace estado duro.
- Por qué `expected` no contradice eso.
- Por qué `Degraded` en vez de rechazar la escritura.
- Por qué el DataNode es quien inicia la conexión.

## 10. Siguiente paso propuesto

Escribir tests que reproduzcan N1, A1, S2 y S1 antes de corregirlos, y luego aplicar los arreglos en ese orden. Los demás hallazgos (A2–A7, P1–P3, R2, N2, S3–S5, H1–H3) quedan como deuda a decidir.
