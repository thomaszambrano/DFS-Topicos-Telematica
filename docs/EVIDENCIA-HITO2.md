# Evidencia de verificación — Hito 2 (DFSha)

**Generado:** 2026-09-26 19:38:23 -05
**Comando:** `make verify` (scripts/verificar.sh)
**Entorno:** Darwin 25.6.0 arm64 · Docker version 29.2.1, build a5c7197 · go1.27.1 · Python 3.14.4

Volúmenes vaciados antes de esta corrida (`docker compose down -v`) para que el
conteo de chunks corresponda exactamente al archivo subido.

## Contenedores en ejecución

```
NAME                SERVICE       STATUS                   PORTS
dfsha-controlnode   controlnode   Up 9 seconds (healthy)   0.0.0.0:8080->8080/tcp, [::]:8080->8080/tcp, 0.0.0.0:9090->9090/tcp, [::]:9090->9090/tcp
dfsha-datanode1     datanode1     Up 9 seconds             0.0.0.0:8081->8080/tcp, [::]:8081->8080/tcp
dfsha-datanode2     datanode2     Up 9 seconds             0.0.0.0:8082->8080/tcp, [::]:8082->8080/tcp
dfsha-datanode3     datanode3     Up 9 seconds             0.0.0.0:8083->8080/tcp, [::]:8083->8080/tcp
dfsha-datanode4     datanode4     Up 9 seconds             0.0.0.0:8084->8080/tcp, [::]:8084->8080/tcp
```

## Corrida de verificación

```

== 0. Estado del clúster
nodo           estado    endpoint                        usado  capacidad  chunks
datanode1      alive     localhost:8081                    0 B    10.0 GB       0
datanode2      alive     localhost:8082                    0 B    10.0 GB       0
datanode3      alive     localhost:8083                    0 B    10.0 GB       0
datanode4      alive     localhost:8084                    0 B    10.0 GB       0
capacidad   : 40.0 GB
usado       : 0 B
libre       : 40.0 GB
nodos vivos : 4
chunks      : 0
chunk_size  : 4.0 MB   R = 1
4 DataNodes vivos

== E.2.1  Crear directorios y listar
creado /demo
tipo    permisos  dueño             tamaño  chunks  modificado           nombre
dir     0755      verificador            -       -  2026-09-27 00:38:24  demo/

== E.2.2  Subir un archivo de 40 MB y observar el reparto
  archivo local : /var/folders/x2/z8p3w_1x3zzfxvg3_91g0ryr0000gn/T/tmp.1cTEgHNZFw/original.bin
  sha256        : f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce

put /var/folders/x2/z8p3w_1x3zzfxvg3_91g0ryr0000gn/T/tmp.1cTEgHNZFw/original.bin -> /demo/prueba.bin
  tamaño      : 40.0 MB (41943040 bytes)
  sha256      : f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce
  chunk_size  : 4.0 MB
  chunks      : 10
  upload_id   : 7c23633c76e9a62bcd64fbbefa48aad3
  version_id  : 39094522f71554eeca3432a7e882ad98

  chunk    0 |    4.0 MB | datanode1@localhost:8081 | 0.04s | b0294db80d0b…
  chunk    1 |    4.0 MB | datanode2@localhost:8082 | 0.03s | 2154ded5154d…
  chunk    2 |    4.0 MB | datanode3@localhost:8083 | 0.03s | e1e94f49b5bc…
  chunk    3 |    4.0 MB | datanode4@localhost:8084 | 0.03s | 29cb5692a2ce…
  chunk    4 |    4.0 MB | datanode1@localhost:8081 | 0.03s | 1b38570cabd0…
  chunk    5 |    4.0 MB | datanode2@localhost:8082 | 0.03s | 15e6fceb6669…
  chunk    6 |    4.0 MB | datanode3@localhost:8083 | 0.02s | a4d90fcda6e6…
  chunk    7 |    4.0 MB | datanode4@localhost:8084 | 0.02s | 16a7d74da7b9…
  chunk    8 |    4.0 MB | datanode1@localhost:8081 | 0.03s | ab41594320f8…
  chunk    9 |    4.0 MB | datanode2@localhost:8082 | 0.03s | 9477e4ba9dc9…

  commit ok | version=39094522f71554eeca3432a7e882ad98 | 10 chunks | 40.0 MB en 0.3s (128.3 MB/s)

== Distribución registrada por el ControlNode (stat, RC-06)
ruta        : /demo/prueba.bin
tipo        : object
dueño       : verificador   permisos: 0644
creado      : 2026-09-27 00:38:24
modificado  : 2026-09-27 00:38:24
tamaño      : 40.0 MB (41943040 bytes)
version_id  : 39094522f71554eeca3432a7e882ad98
sha256      : f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce
chunks      : 10
  [   0] 8cc87bbbf7d9711f281794c83f7b085c |    4.0 MB | datanode1
  [   1] 8e65936d333a9f159b83f23fc6747aef |    4.0 MB | datanode2
  [   2] 2ea27b2bc78a2ef7ff43e2bb80aa3d3d |    4.0 MB | datanode3
  [   3] 20c3aa9490c61deb0be64e90b646805e |    4.0 MB | datanode4
  [   4] 4b11eeb17f04d75fc216f1b9142bbd78 |    4.0 MB | datanode1
  [   5] 4d55cd8daa3a3bb4b6ae5e8d12d0e0ea |    4.0 MB | datanode2
  [   6] 64d6cdf1557f074a07057838d2aa00e8 |    4.0 MB | datanode3
  [   7] ea2da100d57192f58a308d7da0e8705e |    4.0 MB | datanode4
  [   8] 905160eb9508571d636455427f653056 |    4.0 MB | datanode1
  [   9] 785d82b3d2d096b00b602fa4a52f8caa |    4.0 MB | datanode2

== E.2.3  Chunks presentes en el volumen de cada DataNode
Esta es la evidencia de RX-11 y RX-12: cada nodo guarda chunks DISTINTOS.

  datanode1   3 chunks
      4b11eeb17f04d75fc216f1b9142bbd78
      8cc87bbbf7d9711f281794c83f7b085c
      905160eb9508571d636455427f653056
  datanode2   3 chunks
      4d55cd8daa3a3bb4b6ae5e8d12d0e0ea
      785d82b3d2d096b00b602fa4a52f8caa
      8e65936d333a9f159b83f23fc6747aef
  datanode3   2 chunks
      2ea27b2bc78a2ef7ff43e2bb80aa3d3d
      64d6cdf1557f074a07057838d2aa00e8
  datanode4   2 chunks
      20c3aa9490c61deb0be64e90b646805e
      ea2da100d57192f58a308d7da0e8705e

  total de chunks en disco : 10
  nodos que recibieron     : 4 de 4
OK: el archivo está distribuido entre 4 DataNodes

== E.2.4  Descargar y comparar los sha256
get /demo/prueba.bin -> /var/folders/x2/z8p3w_1x3zzfxvg3_91g0ryr0000gn/T/tmp.1cTEgHNZFw/recuperado.bin
  version_id  : 39094522f71554eeca3432a7e882ad98
  tamaño      : 40.0 MB (41943040 bytes)
  chunks      : 10 | hilos: 4
  sha256 esp. : f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce

  chunk    3 |    4.0 MB | localhost:8084 | 0.04s
  chunk    0 |    4.0 MB | localhost:8081 | 0.05s
  chunk    2 |    4.0 MB | localhost:8083 | 0.06s
  chunk    1 |    4.0 MB | localhost:8082 | 0.06s
  chunk    4 |    4.0 MB | localhost:8081 | 0.04s
  chunk    5 |    4.0 MB | localhost:8082 | 0.04s
  chunk    7 |    4.0 MB | localhost:8084 | 0.05s
  chunk    6 |    4.0 MB | localhost:8083 | 0.05s
  chunk    8 |    4.0 MB | localhost:8081 | 0.05s
  chunk    9 |    4.0 MB | localhost:8082 | 0.05s

  ok | sha256 verificado | 40.0 MB en 0.2s (245.0 MB/s)

  original    : f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce
  recuperado  : f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce
OK: integridad verificada extremo a extremo (RT-11)

== Resumen
  Clúster            : 1 ControlNode + 4 DataNodes vivos          (RX-06)
  Archivo            : 40 MB en 10 chunks
  Reparto            : 4 nodos con chunks distintos     (RX-11, RX-12)
  Integridad         : sha256 idéntico                          (RT-11, RT-02)
  Datos por el control: NO, solo metadatos                      (RQ-13)
  Descubrimiento     : el cliente solo conoce http://localhost:8080          (RX-14, RQ-25)
HITO 2 VERIFICADO
```

## Registro del ControlNode: asignación de cada chunk (RC-08)

Cada línea `CHUNK` muestra el índice, el tamaño y el DataNode destino que
el ControlNode eligió. El cliente nunca elige destino (RT-03).

```
2026/09/27 00:38:24.487543 OPEN  /demo/prueba.bin | upload=7c23633c76e9a62bcd64fbbefa48aad3 version=39094522f71554eeca3432a7e882ad98 | 41943040 bytes en 10 chunks de 4 MB
2026/09/27 00:38:24.490526 CHUNK /demo/prueba.bin | idx=0 size=4194304 | chunk=8cc87bbbf7d9711f281794c83f7b085c -> [{datanode1 localhost:8081}]
2026/09/27 00:38:24.538877 CHUNK /demo/prueba.bin | idx=1 size=4194304 | chunk=8e65936d333a9f159b83f23fc6747aef -> [{datanode2 localhost:8082}]
2026/09/27 00:38:24.568062 CHUNK /demo/prueba.bin | idx=2 size=4194304 | chunk=2ea27b2bc78a2ef7ff43e2bb80aa3d3d -> [{datanode3 localhost:8083}]
2026/09/27 00:38:24.596080 CHUNK /demo/prueba.bin | idx=3 size=4194304 | chunk=20c3aa9490c61deb0be64e90b646805e -> [{datanode4 localhost:8084}]
2026/09/27 00:38:24.626895 CHUNK /demo/prueba.bin | idx=4 size=4194304 | chunk=4b11eeb17f04d75fc216f1b9142bbd78 -> [{datanode1 localhost:8081}]
2026/09/27 00:38:24.658959 CHUNK /demo/prueba.bin | idx=5 size=4194304 | chunk=4d55cd8daa3a3bb4b6ae5e8d12d0e0ea -> [{datanode2 localhost:8082}]
2026/09/27 00:38:24.688430 CHUNK /demo/prueba.bin | idx=6 size=4194304 | chunk=64d6cdf1557f074a07057838d2aa00e8 -> [{datanode3 localhost:8083}]
2026/09/27 00:38:24.714011 CHUNK /demo/prueba.bin | idx=7 size=4194304 | chunk=ea2da100d57192f58a308d7da0e8705e -> [{datanode4 localhost:8084}]
2026/09/27 00:38:24.740898 CHUNK /demo/prueba.bin | idx=8 size=4194304 | chunk=905160eb9508571d636455427f653056 -> [{datanode1 localhost:8081}]
2026/09/27 00:38:24.771122 CHUNK /demo/prueba.bin | idx=9 size=4194304 | chunk=785d82b3d2d096b00b602fa4a52f8caa -> [{datanode2 localhost:8082}]
2026/09/27 00:38:24.799492 COMMIT /demo/prueba.bin | version=39094522f71554eeca3432a7e882ad98 | 10 chunks | 41943040 bytes | sha256=f6b49bdf4b39b66a6deb8236a6b83c533bcb73e3c68ca398970cf6499ab986ce
```

## Registro de un DataNode: recepción de chunks

```
2026/09/27 00:38:14.565419 datanode datanode1 | data=/data | listen=:8080 | anuncia=localhost:8081 | control=controlnode:9090 | capacidad=10240 MB
2026/09/27 00:38:14.565521 datanode datanode1: 0 chunks en disco, 0 de 10737418240 bytes ocupados
2026/09/27 00:38:14.565590 [datanode1] heartbeat hacia controlnode:9090 cada 3s
2026/09/27 00:38:14.565670 datanode datanode1 escuchando en :8080
2026/09/27 00:38:24.534809 [datanode1] PUT 8cc87bbbf7d9711f281794c83f7b085c ok: 4194304 bytes en 42ms
2026/09/27 00:38:24.655757 [datanode1] PUT 4b11eeb17f04d75fc216f1b9142bbd78 ok: 4194304 bytes en 28ms
2026/09/27 00:38:24.766890 [datanode1] PUT 905160eb9508571d636455427f653056 ok: 4194304 bytes en 25ms
```
