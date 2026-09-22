# Requisitos del Proyecto — DFSha

**ST0263 Tópicos Especiales en Telemática / SI3007 Sistemas Distribuidos — 2026-2**

**Integrantes:** Camilo Ruiz · Carlos Ochoa · Thomas Osorio

Este documento enumera todo lo que hay que construir para cumplir con DFSha, uniendo el
enunciado del proyecto, las decisiones de diseño (Hito 1) y el documento de requisitos de
implementación.

## Convenciones de prioridad

| Nivel | Significado |
| --- | --- |
| **M** (Must) | Sin esto el sistema no cumple el enunciado. No es negociable. |
| **S** (Should) | Necesario para una implementación correcta; se sacrifica solo ante falta grave de tiempo. |
| **C** (Could) | Mejora real, primera en descartarse. |

---

## 0. Restricciones no negociables (RX)

Condiciones impuestas por el profesor y el enunciado. Todo lo demás debe respetarlas.

- **RX-01** Servidor en **Go** (no C, Rust ni Python).
- **RX-02** Cliente en **Python**.
- **RX-03** Comunicación entre nodos obligatoriamente **RPC (gRPC)**, no REST.
- **RX-04** Cliente ↔ ControlNode como **API bien definida (REST)**.
- **RX-05** Contrato de API con métodos, endpoints, atributos de entrada y salida. "REST sobre HTTP" no basta.
- **RX-06** Clúster mínimo de **3–4 nodos**.
- **RX-07** Factor de replicación **configurable** (no constante); el clúster crece y encoge **en caliente** (el profesor añadirá o quitará nodos en vivo, incluso por debajo de R).
- **RX-08** Interfaz del cliente **de consola** (una web es aceptable pero no la sustituye).
- **RX-09** Cada nodo se ejecuta nativo o en **Docker** y expone su propia API.
- **RX-10** Corre **sobre Internet**, desplegado en la nube con máquinas virtuales.
- **RX-11** Escritura **y** lectura distribuidas entre varios nodos.
- **RX-12** Cada archivo distribuido entre varios nodos (no solo replicado).
- **RX-13** Cada usuario ve **solo sus archivos** salvo permiso explícito.
- **RX-14** Acceso del cliente **dinámico**; prohibida toda configuración estática de localización.
- **RX-15** Entregables: código en repositorio documentado + video (no sustituye la sustentación).

---

## 1. Requisitos del Cliente (RC) — CLI + SDK en Python

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RC-01 | M | Shell interactivo con sesión autenticada y directorio de trabajo (`dfsha shell`) | Se autentica una vez y emite múltiples comandos sin reautenticar. |
| RC-02 | M | `ls [ruta]` con tipo, tamaño, dueño, permisos y fecha | Listar 1.000 entradas responde en menos de 1 s. |
| RC-03 | M | `cd` con rutas relativas, absolutas, `.` y `..` | El estado reside en el cliente; el ControlNode no guarda directorio de trabajo. |
| RC-04 | M | `mkdir` y `rmdir` | `mkdir` falla si el padre no existe o el nombre está ocupado; `rmdir` falla si hay hijos. |
| RC-05 | M | `rm` sobre objetos | El objeto desaparece del namespace; sus chunks quedan marcados para recolección. |
| RC-06 | S | `stat` con tamaño, versión vigente, nº de chunks y réplicas reales por chunk | Permite verificar en vivo la distribución de un archivo entre DataNodes. |
| RC-07 | S | `mv` de renombrado y movimiento | El tiempo de ejecución es independiente del tamaño del objeto. |
| RC-08 | M | Registro visible del particionamiento y transferencia durante `put` | La consola muestra nº de chunks, tamaño y DataNode destino de cada uno. |
| RC-09 | S | Descubrimiento dinámico: solo conoce los endpoints del plano de control | Ningún archivo de configuración del cliente contiene direcciones de DataNodes. |
| RC-10 | S | Selección de réplica por latencia medida, con reparto aleatorio ante empate | Ante caída de una réplica, la descarga continúa contra otra sin error visible. |
| RC-11 | S | Reintento con retroceso exponencial ante errores reintentables | Un `409` de conflicto nunca se reintenta automáticamente. |
| RC-12 | S | SDK en Python con API tipo archivo, independiente de la CLI | Un programa externo hace `put`/`get` importando el SDK, sin usar la consola. |
| RC-13 | C | Comandos administrativos: `df`, `nodes`, `versions` | Permiten inspeccionar ocupación por nodo y estado de membresía. |

---

## 2. Requisitos de transferencia y acceso (RT)

### Particionamiento y escritura

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RT-01 | M | Fragmentar en chunks de tamaño configurable, base **64 MB** | Un archivo de 15 GB produce 240 chunks. |
| RT-02 | M | El último chunk conserva tamaño variable y no se rellena | El SHA-256 del archivo reconstruido coincide con el original. |
| RT-03 | M | El ControlNode define el esquema y el destino de cada chunk; el cliente solo ejecuta | El cliente nunca elige a qué DataNode escribir. |
| RT-04 | M | Los chunks de un mismo archivo se distribuyen entre varios DataNodes | Con 4+ nodos, ningún archivo > 256 MB queda en un solo nodo. |
| RT-05 | M | Replicación en **pipeline**: el cliente envía cada chunk una vez y la cadena lo propaga | El tráfico de subida es ≈ al tamaño del archivo, no R veces. |
| RT-06 | M | Confirmación por quórum de escritura **W = 2** (base) | Con una réplica destino caída, la escritura continúa y se completa en segundo plano. |
| RT-07 | M | El commit de la versión es la última operación y es **atómica** | Un lector concurrente ve la versión anterior íntegra hasta que el commit retorna. |
| RT-08 | S | Cabecera de **idempotencia** en toda escritura | Un reintento tras timeout no produce una versión duplicada. |
| RT-09 | S | Abortar una carga libera el lease y marca los chunks escritos para recolección | Tras abortar, el objeto anterior permanece intacto y accesible. |

### Lectura

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RT-10 | M | `get` descarga los chunks en paralelo y reensambla en orden | El rendimiento agregado de lectura crece al aumentar el nº de DataNodes. |
| RT-11 | M | Verificación de integridad extremo a extremo | `sha256sum` del recuperado coincide con el original, comprobado sobre 15 GB. |
| RT-12 | S | Lectura por rango: solo se descargan los chunks que cubren el rango | `read(offset, len)` dentro de un chunk genera exactamente una petición. |
| RT-13 | S | El cliente verifica el checksum de cada chunk antes de reensamblar | Un chunk corrupto se detecta antes de escribir y se reintenta contra otra réplica. |

### Primitivas de acceso (RF3)

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RT-14 | M | `open`: sesión lógica, valida permisos y entrega manifiesto y tokens | El manifiesto se emite en runtime y tiene vigencia limitada. |
| RT-15 | M | `close`: ejecuta el commit y libera el lease | — |
| RT-16 | M | `read(offset, len)` sobre rangos arbitrarios, incluidos los que cruzan chunks | Un rango que abarca dos chunks descarga exactamente esos dos. |
| RT-17 | M | `write` como carga multiparte de una versión nueva | Una escritura en offset arbitrario sobre una versión existente se rechaza con error. |
| RT-18 | M | `lock` como lease con expiración sobre la clave del objeto | Si el titular desaparece, el lease vence y el objeto vuelve a estar disponible. |

---

## 3. Requisitos del ControlNode (RN)

### Metadatos y namespace

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RN-01 | M | Árbol de namespace con directorios de primera clase (dueño, grupo, permisos, ACL) | `mkdir` crea una entidad persistente con permisos asignables. |
| RN-02 | M | Mapa objeto → versión vigente, y versión → lista ordenada de chunks | El orden de la lista determina el reensamblado. |
| RN-03 | M | Mapa chunk → ubicaciones en memoria, reconstruido desde los inventarios reportados | Tras reiniciar, las ubicaciones se repueblan sin leer disco propio. |
| RN-04 | M | Write-ahead log con **fsync** antes de confirmar, más snapshot periódico | Terminación abrupta a mitad de operaciones; al reiniciar el namespace está íntegro. |
| RN-05 | M | **Modo seguro** al arrancar: atiende lecturas pero no re-replica hasta recibir inventarios | Un reinicio del maestro no dispara copias masivas. |
| RN-06 | S | Versionado inmutable | Una escritura fallida deja el objeto anterior accesible e intacto. |
| RN-07 | S | Recolector de chunks huérfanos | Los chunks de una carga abortada desaparecen tras un ciclo de recolección. |

### Colocación

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RN-08 | M | Invariante dura: dos réplicas del mismo chunk nunca en el mismo DataNode | Verificable con `stat` sobre cualquier objeto. |
| RN-09 | S | Invariante blanda: chunks consecutivos en nodos distintos | La lectura paraleliza sobre todos los nodos disponibles. |
| RN-10 | M | Heurística con función objetivo: minimizar la varianza de ocupación entre nodos | La asignación no es aleatoria pura ni round-robin. |
| RN-11 | M | Algoritmo de **dos candidatos aleatorios con puntuación** (`score = 0.5·(1−u) + 0.3·(1−carga) + 0.2·dispersión`) | Tras 100 chunks concurrentes, la diferencia de ocupación queda bajo 10%. |

### Membresía y recuperación

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RN-12 | M | Heartbeat iniciado por el DataNode; el ControlNode nunca abre conexión hacia él | Las órdenes viajan en la respuesta al heartbeat. |
| RN-13 | M | Clasificación en tres estados: vivo, sospechoso y muerto (9 s / 9–30 s / >30 s) | Un nodo que reinicia rápido no dispara re-replicación. |
| RN-14 | M | Re-replicación automática priorizando los chunks con una sola copia | Al detener un DataNode, el sistema restaura R réplicas sin intervención. |
| RN-15 | S | Detección y limpieza de sobre-replicación | Un nodo que reaparece tras una partición no deja copias excedentes permanentes. |
| RN-16 | S | Control de ritmo: traslados concurrentes acotados; el rebalanceo cede ante la re-replicación | Con chunks sub-replicados pendientes, el rebalanceo queda suspendido. |

### Elasticidad

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RN-17 | M | Rebalanceo activo al ingresar nodos (dispara >10%, para en 5%) | Agregar un nodo vacío a un clúster desbalanceado redistribuye chunks hacia él. |
| RN-18 | M | Traslado por copia y verificación antes de eliminar el origen | Durante el traslado el chunk está sobre-replicado, nunca sub-replicado. |
| RN-19 | S | Baja planificada con **drenado** | Al drenar, ningún chunk queda por debajo de R en ningún instante. |
| RN-20 | M | Replicación efectiva degradada cuando N < R | Con 2 nodos y R=3 el sistema acepta escrituras y las marca como degradadas. |
| RN-21 | M | Cola de reparación persistente que eleva los chunks degradados al recuperar nodos | Al subir nodos, los chunks degradados alcanzan R sin intervención. |
| RN-22 | M | Piso duro: con un solo DataNode disponible se rechazan las escrituras nuevas | El rechazo indica explícitamente la causa. |
| RN-23 | M | Factor de replicación reconfigurable **en caliente** | Cambiar R de 3 a 4 programa las copias faltantes sin reiniciar. |

### Concurrencia y alta disponibilidad

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RN-24 | M | Lease con expiración por clave de objeto, renovable durante cargas largas | Un cliente que desaparece libera el objeto al vencer el lease. |
| RN-25 | S | Locks de lectura y escritura sobre el camino del namespace | Eliminar un directorio mientras se crean hijos no produce estado inconsistente. |
| RN-26 | S | Plano de control replicado con elección de líder (**Raft, 3 nodos**) | La caída del líder no produce pérdida de metadatos confirmados. |
| RN-27 | S | Imposibilidad de split-brain garantizada por quórum de mayoría | Un líder aislado no puede confirmar escrituras de metadatos. |

---

## 4. Requisitos del DataNode (RD)

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RD-01 | M | Almacén local de chunks identificados globalmente, sin conocer el namespace | El DataNode no puede reconstruir a qué archivo pertenece un chunk. |
| RD-02 | M | Checksum calculado en la escritura y verificado en cada lectura | Un chunk alterado en disco se detecta al servirlo y se reporta corrupto. |
| RD-03 | S | Barrido periódico de integridad | Corromper un chunk en disco dispara re-replicación sin intervención. |
| RD-04 | M | Motor de replicación en pipeline: recibe, escribe y reenvía al siguiente | El ack se propaga en sentido inverso hasta el cliente. |
| RD-05 | M | Agente de heartbeat que reporta estado, capacidad, carga e inventario | El ControlNode reconstruye el mapa de ubicaciones solo con estos reportes. |
| RD-06 | M | Ejecuta órdenes recibidas en la respuesta al heartbeat (copiar, eliminar, reportar) | El DataNode nunca recibe conexiones iniciadas por el ControlNode. |
| RD-07 | M | Validación local del token de acceso a chunk antes de servir o aceptar datos | Un token con firma inválida o vencido se rechaza sin consultar al ControlNode. |
| RD-08 | S | Cifrado de cada chunk en reposo (AES-256-GCM) | El contenido del chunk en disco no es legible sin la clave. |
| RD-09 | S | Soporte de lectura parcial mediante cabecera de rango | Permite servir un fragmento sin transferir el chunk completo. |
| RD-10 | S | Endpoint de estado con espacio libre, carga y nº de chunks | Alimenta la función de puntuación de colocación del ControlNode. |
| RD-11 | S | Estado de drenado que rechaza escrituras nuevas mientras evacúa sus chunks | Un nodo en drenado no aparece como destino en ninguna asignación. |
| RD-12 | S | Endpoint anunciable configurable, distinto de la red interna | Un cliente externo alcanza el DataNode aunque la dirección interna sea privada. |

---

## 5. Requisitos no funcionales (RQ)

### Escalabilidad

| Id | P | Requisito | Métrica |
| --- | --- | --- | --- |
| RQ-01 | M | Múltiples usuarios concurrentes | 20 clientes simultáneos sin errores por contención. |
| RQ-02 | M | Archivos de cualquier tamaño, limitado solo por capacidad agregada | Transferencia verificada de un archivo de 15 GB. |
| RQ-03 | S | Validación de identidad sin consulta a base de datos por petición | El token es autoverificable mediante firma. |
| RQ-04 | S | El límite de escalabilidad es la memoria de metadatos del ControlNode, cuantificado | Consumo declarado por objeto y por chunk. |

### Disponibilidad

| Id | P | Requisito | Métrica |
| --- | --- | --- | --- |
| RQ-05 | M | Tolerancia a la pérdida de R−1 DataNodes sin pérdida de datos | Con R=3, detener 2 nodos no impide recuperar ningún archivo. |
| RQ-06 | M | Restauración automática del factor de replicación tras una caída | El sistema vuelve a R réplicas sin intervención manual. |
| RQ-07 | S | Continuidad del servicio ante caída del nodo líder de control | Indisponibilidad de escritura inferior a 10 s. |
| RQ-08 | S | Las lecturas en curso no se interrumpen por la caída del plano de control | Un `get` iniciado antes de la caída se completa. |

### Consistencia

| Id | P | Requisito | Métrica |
| --- | --- | --- | --- |
| RQ-09 | M | Lectura posterior a escritura confirmada devuelve siempre la versión nueva | Verificable desde un cliente distinto al que escribió. |
| RQ-10 | M | Atomicidad de la publicación de versión | Ninguna lectura observa una versión parcialmente escrita. |
| RQ-11 | M | Escrituras concurrentes sobre la misma clave se resuelven por exclusión, no por fusión | El segundo escritor recibe conflicto explícito (`409`). |

### Rendimiento

| Id | P | Requisito | Métrica |
| --- | --- | --- | --- |
| RQ-12 | M | Concurrencia en el acceso a archivos distintos y a datos dentro del mismo archivo | Varios clientes leen chunks distintos del mismo objeto simultáneamente. |
| RQ-13 | M | Los datos no atraviesan el ControlNode en ningún flujo | El tráfico del plano de control es proporcional al nº de chunks, no a su tamaño. |
| RQ-14 | S | El rendimiento agregado de lectura crece al agregar DataNodes | Tendencia creciente medida con 2, 4 y 6 nodos. |

### Seguridad

| Id | P | Requisito | Métrica |
| --- | --- | --- | --- |
| RQ-15 | M | Autenticación obligatoria en toda operación | Ninguna operación procede sin identidad verificada. |
| RQ-16 | M | Contraseñas con función de derivación resistente y salt por usuario (**Argon2id**) | Nunca en texto plano ni con hash simple. |
| RQ-17 | M | Control de acceso por usuario y grupo sobre objetos y directorios | Un usuario no lista ni accede a objetos ajenos sin permiso explícito. |
| RQ-18 | M | Autorización en el plano de datos mediante token firmado de vigencia corta | Un token falsificado o vencido es rechazado por el DataNode. |
| RQ-19 | M | Cifrado en tránsito en todos los enlaces (TLS) | Ningún enlace transporta datos ni credenciales en claro. |
| RQ-20 | M | Autenticación mutua entre nodos de datos y de control (**mTLS**) | Un nodo sin certificado válido no se incorpora al clúster. |
| RQ-21 | S | Cifrado de datos en reposo | El contenido en disco es ilegible sin la clave. |
| RQ-22 | C | Segundo factor de autenticación y claves de API con ámbito | — |
| RQ-23 | S | Registro de auditoría con identidad, operación, recurso, resultado y timestamp | Permite reconstruir quién hizo qué y cuándo. |

### Transparencia

| Id | P | Requisito | Métrica |
| --- | --- | --- | --- |
| RQ-24 | M | Transparencia de localización: la ruta no revela ubicación física | Ninguna ruta del namespace codifica identidad de nodo. |
| RQ-25 | M | Descubrimiento dinámico, sin configuración estática de localización | El cliente opera tras cambiar todas las direcciones de DataNodes, sin cambiar su config. |
| RQ-26 | S | Transparencia de fallos: la caída de una réplica no interrumpe la operación | El reintento contra otra réplica es invisible para el usuario. |
| RQ-27 | C | Montaje del namespace como sistema de archivos local (FUSE) | Extensión opcional. |

---

## 6. Requisitos de interfaz y protocolos (RI)

| Id | P | Requisito | Criterio de aceptación |
| --- | --- | --- | --- |
| RI-01 | M | Contrato completo de la API del cliente (método, ruta, parámetros, respuesta, códigos de error) | Un desarrollador ajeno implementa un cliente alterno usando solo el contrato. |
| RI-02 | M | Toda comunicación entre nodos usa RPC con esquema tipado | Ningún enlace interno emplea REST. |
| RI-03 | M | Los cinco enlaces del sistema están especificados | Cada enlace declara protocolo, dirección de iniciación y mecanismo de seguridad. |
| RI-04 | S | Transferencia de chunks por flujo, sin cargar el chunk completo en memoria | El consumo de memoria del cliente no crece con el tamaño del archivo. |
| RI-05 | S | Catálogo uniforme de errores con indicación de si es reintentable | Los errores de conflicto se distinguen de los transitorios. |
| RI-06 | S | Versionado del contrato de la API mediante prefijo de ruta (`/v1`) | — |
| RI-07 | S | El manifiesto de lectura se emite bajo demanda y caduca | Un manifiesto vencido obliga a solicitar uno nuevo. |
| RI-08 | S | Endpoint de métricas por nodo | Expone latencia, rendimiento, chunks sub-replicados y nodos vivos. |

### Enlaces del sistema

| Enlace | Protocolo | Quién inicia | Seguridad |
| --- | --- | --- | --- |
| Cliente ↔ ControlNode | REST sobre HTTP | Cliente | TLS + token de sesión (JWT) |
| Cliente ↔ DataNode | HTTP con flujo | Cliente | TLS + token de chunk |
| ControlNode ↔ ControlNode | RPC del protocolo de consenso (gRPC/Raft) | Cualquiera | Autenticación mutua (mTLS) |
| ControlNode ↔ DataNode | RPC sobre heartbeat (gRPC) | **Siempre el DataNode** | Autenticación mutua (mTLS) |
| DataNode ↔ DataNode | RPC con flujo (gRPC) | El anterior de la cadena | Autenticación mutua (mTLS) |

---

## 7. Seguridad — mecanismos concretos

- Contraseña con **Argon2id** y salt por usuario; **TOTP** como segundo factor; sesión con **JWT** firmado de 15 min más refresco; **API keys** con ámbito para acceso programático; **mTLS** entre nodos.
- El JWT es autoverificable: el ControlNode valida la firma sin consultar la base de datos.
- Autorización **POSIX extendida con ACL**, evaluada siempre en el ControlNode (recorrer una ruta exige permiso de ejecución sobre cada directorio del camino).
- **Token de acceso a chunk**: `{chunk_id, user_id, permiso, expira_en, term}` firmado con HMAC-SHA256 bajo clave compartida con los DataNodes; validado localmente, sin consultar al ControlNode; incluye el término de Raft para que un líder obsoleto no autorice nada.
- **TLS 1.3** (cliente ↔ servicio), **mTLS** con CA propia (entre nodos), **AES-256-GCM** por chunk en reposo.
- **Cifrado en sobre**: cada objeto recibe una DEK aleatoria que cifra sus chunks; la DEK se guarda cifrada bajo una KEK en los metadatos de la versión; la KEK rota sin recifrar los datos.
- **Auditoría append-only**: identidad, operación, recurso, resultado, IP y marca de tiempo por cada operación.

---

## 8. Exclusiones explícitas

Fuera de alcance por decisión de diseño. No son omisiones.

- Actualización parcial in-place de un objeto.
- Operación de anexado (append) a un objeto existente.
- Transacciones que abarquen varios objetos.
- Instantánea consistente de un directorio.
- Renombrado atómico de directorios con gran número de descendientes.
- Enlaces duros y simbólicos.
- Cumplimiento POSIX completo.
- Protección frente a un operador malicioso del propio servicio.

---

## 9. Entregables (RX-15)

1. **Informe técnico**: objetivo y marco teórico, descripción del servicio, arquitectura y diagramas, especificación de protocolos y APIs, algoritmos de particionamiento y distribución, entorno de ejecución, pruebas y análisis de resultados.
2. **Código fuente** en repositorio documentado y reproducible.
3. **Video demostración (10–15 min)**: explicación del sistema y ejecución del procesamiento distribuido.

---

## 10. Núcleo mínimo para cumplir el enunciado

Si el tiempo aprieta, los requisitos **[M]** son el núcleo irrenunciable: sin cualquiera de ellos, el sistema no cumple el enunciado. Los **[S]** se sacrifican solo ante falta grave de tiempo y los **[C]** son los primeros en descartarse.
