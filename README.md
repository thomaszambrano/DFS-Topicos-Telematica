# DFSha — Sistema de archivos distribuido

**ST0263 Tópicos Especiales en Telemática / SI3007 Sistemas Distribuidos — 2026-2**
Camilo Ruiz · Carlos Ochoa · Thomas Osorio — [Informe técnico](INFORME.md) ([PDF](INFORME.pdf)) · [Video demostración](https://youtu.be/No2hkhglwMw)

Servidor en **Go**, cliente en **Python**. Plano de control (ControlNode) y plano de
datos (DataNodes) separados: los metadatos viajan por REST y gRPC, los bytes van
directamente del cliente a los DataNodes y **nunca atraviesan el ControlNode**.

---

## Estado: Hito 3 (parcial)

Sobre la arquitectura del Hito 2 se añadió **replicación con quórum, re-replicación
automática y transferencia gRPC entre DataNodes**.

| Incluido | Pendiente |
| --- | --- |
| Particionamiento en chunks de tamaño configurable | Replicación en pipeline desde el cliente (RT-05) |
| Reparto real entre 4 DataNodes | Rebalanceo al ingresar nodos y drenado |
| Namespace con directorios y objetos | WAL, snapshot y modo seguro |
| Versionado inmutable y commit atómico | Recolección de huérfanos y de réplicas excedentes |
| Membresía por heartbeat y descubrimiento dinámico | Autenticación, ACL y TLS/mTLS |
| Verificación de integridad extremo a extremo | Raft en el plano de control |
| **Replicación R=3 con quórum de escritura W=2** | Heurística de colocación con puntuación (RN-10/11) |
| **Re-replicación automática priorizada** | |
| **Factor de replicación configurable en caliente** | |
| **gRPC con flujo entre DataNodes (`ChunkService`)** | |
| **Failover de lectura entre réplicas** | |
| **Rechazo de escrituras con un solo DataNode vivo** | |

### Limitaciones declaradas

No son omisiones; son alcance pendiente y están documentadas en `DECISIONES.md`.

1. **El namespace vive solo en memoria.** Un reinicio del ControlNode lo pierde y los
   chunks quedan huérfanos. El ControlNode es punto único de falla: el WAL con `fsync`
   (RN-04) y Raft (RN-26) están pendientes.
2. **La replicación no es en pipeline.** El cliente sube cada chunk a sus R destinos, así
   que el tráfico de subida es R veces el archivo. El DataNode ya sabe reenviar en cadena
   (`ChunkService`, campo `downstream`, con test), pero el camino de escritura del
   cliente todavía no lo usa (D-015).
3. **Seguridad básica, sin TLS.** Hay registro y login (Argon2id), token HMAC de 15 min y
   aislamiento por `/home/<usuario>` (D-017). Faltan TLS, tokens por chunk hacia los
   DataNodes y cifrado en reposo; los usuarios viven en memoria como el namespace.
4. **Sin recolección de huérfanos ni de excedentes.** Sobrescribir un objeto deja en disco
   los chunks de la versión anterior; bajar R o recuperar un nodo deja copias de más. La
   orden `DELETE_CHUNK` ya existe en el DataNode, pero el ControlNode aún no la emite.

---

## Arranque rápido

Requisitos: Docker y Python 3.10+. Para compilar sin contenedores, Go 1.22+.

```bash
make up        # construye e inicia 1 ControlNode + 4 DataNodes
make verify    # verificación completa del hito (E.2): reparto + integridad
make shell     # consola interactiva del cliente
make down      # detiene el clúster
make clean     # lo detiene y borra los volúmenes
```

`make verify` es la prueba de la entrega: crea un archivo de 40 MB, lo sube, muestra
el reparto, **entra a los cuatro volúmenes y lista los chunks de cada nodo**, lo
descarga en paralelo y compara los `sha256sum`.

### Prueba de alta disponibilidad

```bash
make up && make verify
docker stop dfsha-datanode2       # simula la caída de un nodo
docker logs -f dfsha-controlnode  # tras ~30 s (DEAD_AFTER) aparecen líneas REPARAR
make shell                        # get del archivo: sigue funcionando desde otras réplicas
```

Cambiar el factor de replicación sin reiniciar (RX-07, RN-23):

```bash
curl -X PUT localhost:8080/v1/config/replication -d '{"replication_factor": 4}'
```

### Sin Docker

```bash
./scripts/correr-nativo.sh iniciar    # 1 ControlNode + 4 DataNodes como procesos
./scripts/correr-nativo.sh estado
./scripts/correr-nativo.sh detener
```

### Consola

```
$ make shell
dfsha:/$ mkdir /media
dfsha:/$ put ~/video.mp4 /media/video.mp4
dfsha:/$ stat /media/video.mp4      # muestra en qué DataNode vive cada chunk
dfsha:/$ get /media/video.mp4 copia.mp4
dfsha:/$ nodes                      # membresía del clúster
dfsha:/$ df                         # ocupación agregada
```

### Como biblioteca (SDK)

La CLI no es la única interfaz: el SDK se importa y se usa sin consola.

```python
from dfsha import ControlClient, put, get

control = ControlClient("http://localhost:8080", user="thomas")
put(control, "video.mp4", "/media/video.mp4")
get(control, "/media/video.mp4", "copia.mp4")
```

---

## Arquitectura

```
                 REST /v1/*                    gRPC Heartbeat
   Cliente  ──────────────────▶  ControlNode  ◀──────────────────  DataNode
  (Python)   metadatos, planes   (namespace,    el DataNode SIEMPRE  (Go)
      │      y manifiestos        colocación,   inicia la conexión     │
      │                           replicador)   órdenes COPY en la     │ gRPC con flujo
      │                                         respuesta              │ ChunkService
      │                                                                ▼
      └──────────────────────────────────────────────────────────▶  DataNode
                     HTTP con flujo: PUT/GET /chunk/{id}
                        LOS BYTES VAN POR AQUÍ
```

| Enlace | Protocolo | Quién inicia |
| --- | --- | --- |
| Cliente ↔ ControlNode | REST sobre HTTP (`api/openapi.yaml`) | Cliente |
| Cliente ↔ DataNode | HTTP con flujo | Cliente |
| ControlNode ↔ DataNode | gRPC `DataNodeService.Heartbeat` | Siempre el DataNode |
| DataNode ↔ DataNode | gRPC con flujo `ChunkService.ReplicateChunk` | El nodo que envía |
| ControlNode ↔ ControlNode | Pendiente (Raft) | — |

### Las tres reglas que explican el diseño

**1. Los datos no pasan por el ControlNode.** El tráfico del plano de control es
proporcional al *número* de chunks, no a su tamaño. Un archivo de 15 GB genera 240
peticiones de metadatos de unos cientos de bytes cada una.

**2. El DataNode siempre inicia la conexión.** El ControlNode nunca abre una conexión
hacia un DataNode: las órdenes viajan en la respuesta al heartbeat. Eso permite que
los DataNodes vivan detrás de NAT y que el ControlNode no tenga que saber cómo
alcanzarlos.

**3. Las ubicaciones de los chunks no se persisten.** El disco de cada DataNode es el
único que sabe con certeza qué tiene *ahora*. El ControlNode reconstruye el mapa
`chunk → nodos` con los inventarios del heartbeat. El namespace, en cambio, sí es
información que solo él posee: si se pierde, los DataNodes tienen todos los bytes y
ninguno sabe a qué archivo pertenecen. Es la distinción entre **estado blando** y
**estado duro**, y está explicada en `DECISIONES.md` (D-002).

### Secuencia de escritura

```
POST /v1/obj/open                      → upload_id, chunk_size, lease
  por cada chunk:
    POST /v1/obj/{id}/chunk            → chunk_id + R DataNodes destino
    PUT  http://{destino}/chunk/{cid}  → los bytes, directo a cada destino
                                          se anota qué destinos confirmaron (ACK)
POST /v1/obj/{id}/commit               → publica la versión (ATÓMICO)
                                          con los ACK por chunk; exige W por chunk
```

Hasta que `commit` retorna, un lector de la misma ruta ve la versión anterior íntegra.
Publicar es mover un puntero bajo candado: no existe un instante en el que se pueda
observar una versión a medias.

Con R=3 y W=2, la caída de un destino durante la subida no la detiene. El ControlNode
registra como réplicas **solo** los nodos que confirmaron y que él mismo asignó; los
chunks con menos de R copias los completa el replicador en segundo plano.

### Re-replicación

Cada 5 s el replicador busca los chunks de versiones vigentes con menos de R réplicas
vivas, empezando por los que tienen menos copias (RN-14). Elige un origen vivo y un
destino sin copia, ambos con la menor carga, y deja la orden `COPY_CHUNK` en la cola del
origen. El origen la recibe **en la respuesta a su próximo heartbeat** y envía el chunk
al destino por gRPC con flujo; el destino verifica el checksum antes de aceptarlo. Un
nodo sospechoso todavía cuenta como réplica: solo se repara cuando pasa a muerto
(`DEAD_AFTER`), para que un reinicio rápido no dispare copias.

### Secuencia de lectura

```
GET /v1/obj/manifest?path=…            → lista ORDENADA de chunks + réplicas vivas
  en paralelo, por cada chunk:
    GET http://{réplica}/chunk/{cid}   → verificar checksum y escribir en su offset
verificar el sha256 del archivo completo
```

Las descargas van en paralelo y la escritura es ordenada. Los chunks no se acumulan en
memoria: el archivo destino se preasigna y cada hilo escribe en su desplazamiento con
`os.pwrite`. El consumo es `hilos × chunk_size`, no `chunks × chunk_size`.

---

## Estructura

```
api/openapi.yaml            contrato REST cliente ↔ ControlNode
api/dfsha.proto             contrato gRPC entre nodos
internal/types/             tipos del dominio: ChunkMeta, Version, Inode
internal/datanode/          store, checksum, servidor HTTP, heartbeat, ChunkService gRPC
internal/controlnode/       registry, namespace, placement, allocator, replicador, servidor
cmd/{controlnode,datanode}/ ejecutables
client/dfsha/               chunker, SDK, transferencia, consola
docker/                     imágenes multietapa
scripts/verificar.sh        verificación del hito (E.2)
INFORME.md                  informe técnico: objetivos, arquitectura, pruebas, resultados
DECISIONES.md               bitácora de decisiones de diseño
```

---

## Configuración

Todo por variables de entorno; no hay archivos de configuración con direcciones.

### ControlNode

| Variable | Por omisión | Qué hace |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | API REST del cliente |
| `GRPC_ADDR` | `:9090` | RPC de heartbeats |
| `CHUNK_SIZE` | `67108864` | Tamaño de chunk (64 MB). En la demo se baja a 4 MB |
| `REPLICATION_FACTOR` | `1` | R. `docker-compose.yml` usa 3. Se cambia en caliente con `PUT /v1/config/replication` |
| `WRITE_QUORUM` | `2` | W: confirmaciones mínimas por chunk. Se acota por los destinos asignados |
| `SUSPECT_AFTER` | `9s` | Sin reportar, el nodo pasa a sospechoso |
| `DEAD_AFTER` | `30s` | Sin reportar, el nodo se da por muerto |
| `LEASE_TTL` | `2m` | Vigencia del lease sobre la clave de un objeto |

### DataNode

| Variable | Por omisión | Qué hace |
| --- | --- | --- |
| `NODE_ID` | — (obligatorio) | Identidad del nodo |
| `DATA_DIR` | `/data` | Dónde viven los chunks |
| `LISTEN_ADDR` | `:8080` | Dirección de escucha |
| `ADVERTISED_ENDPOINT` | = `LISTEN_ADDR` | Dirección que se anuncia a los clientes |
| `CONTROL_ADDR` | — (obligatorio) | ControlNode al que reporta |
| `CAPACITY` | `10 GiB` | Capacidad declarada |
| `HEARTBEAT_INTERVAL` | `3s` | Ritmo de reporte |
| `GRPC_ADDR` | `:9091` | Escucha del `ChunkService` (DataNode ↔ DataNode) |
| `GRPC_ENDPOINT` | `localhost` + `GRPC_ADDR` | Dirección gRPC que se anuncia a los demás DataNodes |

`ADVERTISED_ENDPOINT` es distinto de `LISTEN_ADDR` a propósito: dentro de Docker el
nodo escucha en `:8080` pero anuncia `localhost:8081`, que es la dirección por la que
un cliente del host lo alcanza. `GRPC_ENDPOINT`, en cambio, es una dirección **entre
nodos**: dentro de Docker es el nombre del servicio (`datanode2:9091`), porque
`localhost` solo existe para quien está en el host.

En `docker-compose.yml` los DataNodes publican los puertos 8081, 8085, 8083 y 8084.

### Cliente

| Variable | Por omisión |
| --- | --- |
| `DFSHA_CONTROL` | `http://localhost:8080` |
| `DFSHA_USER` | `$USER` |

El cliente **solo** conoce la dirección del ControlNode. Las de los DataNodes llegan
en las respuestas, en tiempo de ejecución.

---

## Desarrollo

```bash
make tools    # instala protoc-gen-go y protoc-gen-go-grpc
make proto    # regenera internal/pb/ desde api/dfsha.proto
make build    # compila los binarios en bin/
make test     # pruebas unitarias
make vet      # análisis estático
make fmt      # formato
```
