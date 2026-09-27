# DECISIONES.md — Bitácora de decisiones de diseño

Cada decisión no obvia queda registrada aquí. Este es el documento que leemos la
noche antes de la sustentación.

**Formato:** qué elegimos, qué descartamos, y por qué. El *por qué* es lo único
que el profesor va a preguntar.

---

## D-001 — Ruta del módulo de Go

**Fecha:** 2026-09-26
**Elegido:** `github.com/thomaszambrano/DFS-Topicos-Telematica`, igual a la URL del
repositorio.
**Descartado:** un nombre corto como `dfsha`.
**Porqué:** en Go la ruta del módulo es también la ruta de importación, y por
convención coincide con la URL del repositorio para que `go get` funcione sin
configuración extra. Cambiarla después obliga a editar los imports de todos los
archivos. Los imports quedan largos, que es el costo aceptado.

---

## D-002 — `Replicas` no se persiste: es estado blando

**Fecha:** 2026-09-26
**Elegido:** el campo `ChunkMeta.Replicas` lleva `json:"-"` y nunca se escribe al
WAL ni al snapshot. El ControlNode lo reconstruye con los inventarios que los
DataNodes reportan en el heartbeat (RD-05 → RN-03).
**Descartado:** persistirlo junto al resto de los metadatos del chunk.
**Porqué:** el dueño de esa información es el disco de cada DataNode — es el único
que puede responder con certeza si tiene un chunk *ahora mismo*. El ControlNode
solo puede guardar lo que creía en algún momento pasado, y mientras está caído el
clúster sigue cambiando: un nodo puede morir o perder su volumen sin que nadie se
lo anote. Al reiniciar tendría dos opciones y las dos son malas: creerle a su
disco, y entonces entregarle a un cliente la dirección de un chunk que ya no
existe (y peor, en el Hito 3, creer que está replicado y no re-replicarlo nunca);
o verificarlo preguntando, y entonces haberlo guardado no sirvió de nada.

Distinción que aplica: la lista ordenada de chunks de una versión **sí** se
persiste, porque nadie más la conoce. Si se pierde, los DataNodes tienen todos los
bytes y ninguno sabe a qué archivo pertenecen ni en qué orden van (RD-01): el dato
existe y está perdido. El criterio no es quién tiene los bytes, es **si hay
alguien a quien preguntarle**.

Es la misma decisión de GFS (el maestro no persiste las ubicaciones de chunks, las
consulta al arrancar) y de HDFS (BlockReports). De aquí sale la necesidad del modo
seguro, RN-05: si el ControlNode actuara antes de recibir los inventarios,
concluiría que todo está sub-replicado y desataría una tormenta de replicación
sobre un clúster sano.

---

## D-003 — El checksum se guarda como `string` en hexadecimal

**Fecha:** 2026-09-26
**Elegido:** `Checksum string`, 64 caracteres hexadecimales.
**Descartado:** `[]byte` (32 bytes crudos) y `[32]byte` (arreglo de tamaño fijo).
**Porqué:** el hexadecimal es el formato nativo de los tres extremos del sistema —
`sha256sum` en la terminal, `hashlib.hexdigest()` en el cliente Python y
`hex.EncodeToString` en el servidor Go producen la misma cadena. No hay ninguna
conversión entre lenguajes, que es superficie de bugs eliminada. Además permite
verificar la integridad a simple vista en la demo (criterio E.2.4), se compara con
`==` sin necesidad de `bytes.Equal`, es inmutable por ser `string`, y es legible y
greppable en los logs que exige RC-08.

`[]byte` fue descartado por ser la más débil de las tres: asigna en el heap, no se
compara con `==`, no sirve como clave de mapa, es mutable, y `encoding/json` lo
emite en **base64**, que no se puede cotejar con la salida de `sha256sum`.
`[32]byte` es la más eficiente (32 bytes inline, sin asignación ni puntero que el
GC rastree, y es lo que devuelve `sha256.Sum256`), pero en JSON sale como un
arreglo de 32 números.

**El costo, cuantificado:** `string` consume ~80 bytes por chunk contra 32 de
`[32]byte`. Son ~48 MB de RAM adicionales por cada 64 TB administrados —
irrelevante a nuestra escala. GFS sí optimizó esto (~64 bytes de metadatos por
chunk) porque manejaba miles de millones de chunks; nosotros no estamos en ese
régimen. Decidimos por operabilidad, no por memoria, y podemos respaldarlo con la
cuenta (RQ-04).

---

## D-004 — `chunk_size` de 4 MB en la demostración, 64 MB por omisión

**Fecha:** 2026-09-27
**Elegido:** `CHUNK_SIZE` como variable de entorno. El valor por omisión del código es
64 MB (`DefaultChunkSize`); `docker-compose.yml` lo baja a 4 MB.
**Descartado:** dejar 64 MB fijo y hacer la demostración con un archivo de más de
256 MB.
**Porqué:** con 64 MB y 4 DataNodes hacen falta más de 192 MB de archivo para que los
cuatro volúmenes tengan algo que mostrar; con menos, dos nodos quedan vacíos y la
captura de la evidencia (E.2.3) no demuestra nada. Bajarlo a 4 MB no es un atajo: RT-01
exige que el tamaño sea **configurable**, así que la demostración prueba el requisito en
lugar de esquivarlo. Un archivo de 40 MB produce 10 chunks repartidos entre los 4 nodos.

---

## D-005 — `Upload` separado de `Version`

**Fecha:** 2026-09-27
**Elegido:** una estructura `Upload` en el Allocator para la carga en construcción, y
`types.Version` solo para versiones ya publicadas.
**Descartado:** un campo de estado dentro de `Version` (`en_construccion` / `publicada`).
**Porqué:** con estructuras separadas es **imposible** que un lector vea una versión a
medias, porque una versión sin publicar no está en el namespace: no hay nada que
filtrar ni ninguna comprobación de estado que se pueda olvidar. Con un campo de estado,
cada lectura tendría que recordar comprobarlo, y el día que alguien olvide el `if`, el
sistema entrega un archivo incompleto. El commit queda reducido a una asignación de
puntero, que es lo que lo hace atómico (RT-07, RQ-10).

---

## D-006 — El `Inode` guarda solo la versión vigente

**Fecha:** 2026-09-27
**Elegido:** `Inode.Current *Version`. Una sola versión por objeto.
**Descartado:** una lista con el historial completo de versiones.
**Porqué:** el historial no aporta nada a este hito y sí cuesta: obligaría a decidir
cuántas versiones retener, cuándo recolectar los chunks de las viejas y cómo exponerlas
en la API. El campo se llama `Current` y no `Version` para que añadir el historial en el
futuro no implique renombrar nada. `nil` significa "el objeto existe y no tiene contenido
publicado", que es el estado entre `OpenUpload` y `Commit`.

---

## D-007 — El checksum del chunk se guarda en un archivo aparte

**Fecha:** 2026-09-27
**Elegido:** un archivo `<chunk_id>.sum` junto a `<chunk_id>.chunk` en el DataNode.
**Descartado:** recalcular el SHA-256 en cada lectura.
**Porqué:** recalcular cuesta una pasada completa de hash por cada `GET`, lo que
duplica el trabajo de E/S de toda lectura. Con el archivo aparte, servir un chunk es
copiar bytes. El costo es un archivo que puede desincronizarse del contenido, y por eso
existe `Store.Verify`, sobre el que se construirá el barrido de integridad del Hito 3
(RD-03).

---

## D-008 — El código generado por `protoc` se versiona en el repositorio

**Fecha:** 2026-09-27
**Elegido:** `internal/pb/*.pb.go` se sube al repositorio.
**Descartado:** generarlo en cada build y excluirlo con `.gitignore`.
**Porqué:** quien clone el repositorio puede compilar con solo Go instalado, sin
`protoc` ni sus plugins. El costo es que hay que acordarse de correr `make proto` al
cambiar el `.proto`, y que los diffs incluyen código generado.

---

## D-009 — Escritura atómica por renombrado en el DataNode

**Fecha:** 2026-09-27
**Elegido:** escribir en `<id>-XXXX.tmp`, `Sync()`, renombrar al nombre definitivo y
hacer `fsync` del directorio.
**Descartado:** escribir directamente en `<id>.chunk`.
**Porqué:** si el proceso muere a mitad de una escritura directa, queda un archivo
incompleto **con nombre válido**: el sistema lo creería bueno y serviría datos
truncados, que es peor que no tener el chunk. El renombrado dentro del mismo sistema de
archivos es atómico, así que el chunk o existe completo o no existe. El `Sync()` va
antes del renombrado porque si no, el nombre puede quedar visible mientras el contenido
sigue en el caché del sistema operativo.

---

## D-010 — Ubicaciones esperadas de corta vida para la lectura tras escritura

**Fecha:** 2026-09-27
**Elegido:** al confirmar una versión, el Allocator informa al Registry de las
ubicaciones que el cliente declaró haber escrito. Caducan en `SUSPECT_AFTER` y el
primer inventario que llegue de cada nodo las reemplaza.
**Descartado:** depender únicamente de los inventarios del heartbeat.
**Porqué:** se detectó en la verificación del hito. Con solo heartbeats, entre el
`commit` y el siguiente reporte —hasta 3 segundos— el ControlNode no sabe dónde están
los chunks que acaba de asignar, y `stat` mostraba «SIN RÉPLICAS VIVAS» mientras los
chunks estaban en disco. Una lectura inmediata después de una escritura confirmada
fallaba con 503, lo que rompe RQ-09.

Esto **no** contradice D-002: sigue sin persistirse nada, la suposición caduca sola y
la autoridad continúa siendo el disco del DataNode. Solo cubre la ventana en la que el
nodo todavía no ha hablado. GFS hace lo mismo: el maestro sabe a qué chunkservers
ordenó escribir, y los reportes periódicos confirman o corrigen esa suposición.

---

## D-011 — El namespace del Hito 2 vive solo en memoria

**Fecha:** 2026-09-27
**Elegido:** sin WAL ni snapshot. Un reinicio del ControlNode pierde el namespace.
**Descartado:** implementar el WAL con `fsync` desde este hito.
**Porqué:** `HITO2.md` lo admite explícitamente como limitación declarada, y lo que no
se puede sacrificar es el reparto real de chunks y la verificación de integridad. El
WAL es del Hito 3 (RN-04) y su criterio de aceptación —`kill -9` a mitad de operaciones
y namespace íntegro al reiniciar— es una etapa completa por sí misma.

**Consecuencia que hay que saber explicar:** al reiniciar el ControlNode, los chunks
siguen en los DataNodes pero ninguno sabe a qué archivo pertenece. Los datos existen y
están perdidos. Es la demostración práctica de por qué el namespace es estado duro.

---

## D-012 — Un solo candado para el árbol del namespace

**Fecha:** 2026-09-27
**Elegido:** un `sync.RWMutex` que protege el árbol completo.
**Descartado:** candados por nodo o por camino (RN-25).
**Porqué:** a la escala de este proyecto la contención no es el cuello de botella —las
operaciones de namespace son en memoria y duran microsegundos—, mientras que un árbol
con candados por nodo exige tomarlos siempre en el mismo orden para no provocar
interbloqueos, y cualquier operación que cruce dos ramas (como `Rename`) es una fuente
inagotable de errores difíciles de reproducir. Si la contención apareciera, la medida
correcta es medirla antes de cambiarlo.

---

# Decisiones pendientes para el Hito 3

| # | Decisión |
| --- | --- |
| P-1 | Heurística de colocación: dos candidatos aleatorios con puntuación (RN-10, RN-11) |
| P-2 | Replicación en pipeline y quórum de escritura W=2 (RT-05, RT-06) |
| P-3 | WAL con `fsync` + snapshot, y modo seguro al arrancar (RN-04, RN-05) |
| P-4 | Recolector de chunks huérfanos (RN-07) |
| P-5 | Re-replicación priorizada y control de ritmo (RN-14, RN-16) |
| P-6 | Rebalanceo con umbrales 10 % / 5 % y drenado (RN-17, RN-19) |
