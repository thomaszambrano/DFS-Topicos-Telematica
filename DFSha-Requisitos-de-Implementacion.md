# DFSha — Especificación de Requisitos de Implementación

**ST0263 Tópicos Especiales en Telemática / SI3007 Sistemas Distribuidos — 2026-2**

**Integrantes:** Camilo Ruiz · Carlos Ochoa · Thomas Osorio

2026-09-20

## 1. Propósito y convenciones

Este documento traduce las decisiones de diseño de DFSha en requisitos de implementación verificables. Responde a la pregunta operativa: **qué hay que construir exactamente para que el sistema cumpla lo decidido**.

### Identificación

| Prefijo | Ámbito |
| --- | --- |
| `RC-nn` | Cliente |
| `RT-nn` | Transferencia y acceso a datos |
| `RN-nn` | ControlNode |
| `RD-nn` | DataNode |
| `RI-nn` | Interfaces y protocolos |
| `RQ-nn` | No funcionales, con métrica |

### Prioridad

| Nivel | Significado |
| --- | --- |
| **M** (Must) | Sin esto el sistema no cumple el enunciado. No es negociable |
| **S** (Should) | Necesario para una implementación correcta; se sacrifica solo ante falta grave de tiempo |
| **C** (Could) | Mejora real, primera en descartarse |

### Fuente

| Código | Origen |
| --- | --- |
| `E` | Enunciado escrito del proyecto |
| `P` | Indicación explícita del profesor en la presentación |
| `D` | Derivado de una decisión de diseño propia del equipo |

### Formato

Cada requisito lleva identificador, prioridad, fuente, enunciado y **criterio de aceptación**: la condición observable que permite afirmar que está cumplido. Un requisito sin criterio verificable no pertenece a este documento.

## 2. Restricciones no negociables

Condiciones impuestas desde fuera del equipo. No admiten decisión propia y cualquier diseño debe respetarlas.

| Id | Restricción | Fuente |
| --- | --- | --- |
| RX-01 | El servidor se implementa en Go. C y Rust fueron descartados por bajo nivel; Python no es opción para el backend | P |
| RX-02 | El cliente se implementa en Python | P |
| RX-03 | La comunicación entre nodos es obligatoriamente RPC, no REST | P |
| RX-04 | La comunicación cliente a ControlNode se expone como una API bien definida, normalmente REST | P |
| RX-05 | El contrato de la API debe especificar métodos, endpoints y atributos de entrada y salida. Declarar REST sobre HTTP no constituye especificación | P |
| RX-06 | El clúster opera con un mínimo de 3 a 4 nodos | P |
| RX-07 | El factor de replicación es un parámetro configurable, no una constante. El clúster debe poder crecer y encogerse en caliente | P |
| RX-08 | La interfaz del cliente es de consola; una interfaz web es aceptable pero no la sustituye | P |
| RX-09 | Cada nodo se ejecuta nativamente o en contenedor Docker, y expone su propia API | E |
| RX-10 | El sistema corre sobre Internet, desplegado en AWS o GCP con máquinas virtuales | E, P |
| RX-11 | Tanto la escritura como la lectura deben ser distribuidas entre varios nodos | E |
| RX-12 | Cada archivo debe estar distribuido entre varios nodos, no solo replicado | E |
| RX-13 | Cada usuario ve únicamente sus propios archivos, salvo permiso explícito | E, P |
| RX-14 | El mecanismo de acceso del cliente debe ser dinámico; queda prohibida toda configuración estática de localización | E |
| RX-15 | Entregables: código fuente en repositorio documentado. El video no sustituye la sustentación | E, P |

Sobre RX-07: el profesor anunció que puede agregar o retirar nodos durante la sustentación, incluso dejando el clúster por debajo del factor de replicación configurado. El sistema debe responder a eso en vivo, no solo tolerarlo en teoría.

## 3. Requisitos del Cliente

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RC-01 | M | E | Shell interactivo de consola que mantiene sesión autenticada y directorio de trabajo | Se ejecuta `dfsha shell`, se autentica una vez y se emiten múltiples comandos sin volver a autenticar |
| RC-02 | M | E | Comando `ls [ruta]` que lista hijos con tipo, tamaño, dueño, permisos y fecha | Listar un directorio con 1.000 entradas responde en menos de 1 s |
| RC-03 | M | E | Comando `cd` con soporte de rutas relativas, absolutas, `.` y `..` | El estado reside en el cliente; el ControlNode no almacena directorio de trabajo |
| RC-04 | M | E | Comandos `mkdir` y `rmdir` | `mkdir` falla si el padre no existe o el nombre está ocupado; `rmdir` falla con error explícito si hay hijos |
| RC-05 | M | E | Comando `rm` sobre objetos | El objeto desaparece del namespace; sus chunks quedan marcados para recolección |
| RC-06 | S | D | Comando `stat` que reporta tamaño, versión vigente, número de chunks y réplicas reales por chunk | Permite verificar en vivo la distribución de un archivo entre DataNodes |
| RC-07 | S | D | Comando `mv` de renombrado y movimiento dentro del namespace | El tiempo de ejecución es independiente del tamaño del objeto |
| RC-08 | M | P | Registro visible en el cliente del proceso de particionamiento y transferencia | Durante `put`, la consola muestra número de chunks, tamaño y DataNode destino de cada uno |
| RC-09 | S | D | Descubrimiento dinámico: el cliente arranca conociendo solo los endpoints del plano de control | Ningún archivo de configuración del cliente contiene direcciones de DataNodes |
| RC-10 | S | D | Selección de réplica por latencia medida, con reparto aleatorio ante empate | Ante caída de una réplica, la descarga continúa contra otra sin error visible |
| RC-11 | S | D | Reintento con retroceso exponencial ante errores marcados como reintentables | Un `409` de conflicto nunca se reintenta de forma automática |
| RC-12 | S | E | SDK en Python con API tipo archivo, independiente de la CLI | Un programa externo realiza `put` y `get` importando el SDK, sin invocar la consola |
| RC-13 | C | D | Comandos administrativos: `df`, `nodes`, `versions` | Permiten inspeccionar ocupación por nodo y estado de membresía |

## 4. Requisitos de transferencia y acceso

### Particionamiento y escritura

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RT-01 | M | E | El cliente fragmenta el archivo en chunks de tamaño configurable, con valor base 64 MB | Un archivo de 15 GB produce 240 chunks |
| RT-02 | M | D | El último chunk conserva tamaño variable y no se rellena | El SHA-256 del archivo reconstruido coincide con el original para tamaños que no son múltiplo del chunk |
| RT-03 | M | P | El ControlNode define el esquema de particionamiento y el destino de cada chunk; el cliente ejecuta la transferencia | El cliente nunca elige por su cuenta a qué DataNode escribir |
| RT-04 | M | E | Los chunks de un mismo archivo se distribuyen entre varios DataNodes | Con 4 o más nodos activos, ningún archivo mayor a 256 MB queda alojado en un solo nodo |
| RT-05 | M | D | Replicación en pipeline: el cliente envía cada chunk una vez y la cadena lo propaga | El tráfico de subida medido en el cliente es aproximadamente igual al tamaño del archivo, no a R veces |
| RT-06 | M | D | Confirmación por quórum de escritura W, con valor base 2 | Con una réplica destino caída, la escritura continúa y se completa en segundo plano |
| RT-07 | M | D | El commit de la versión es la última operación y es atómica | Un lector concurrente ve la versión anterior íntegra hasta que el commit retorna |
| RT-08 | S | D | Cabecera de idempotencia en toda operación de escritura | Un reintento tras timeout de red no produce una versión duplicada |
| RT-09 | S | D | Abortar una carga libera el lease y marca los chunks escritos para recolección | Tras abortar, el objeto anterior permanece intacto y accesible |

### Lectura

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RT-10 | M | E | `get` descarga los chunks en paralelo y reensambla en orden | El rendimiento agregado de lectura crece al aumentar el número de DataNodes |
| RT-11 | M | P | Verificación de integridad extremo a extremo | `sha256sum` del archivo recuperado coincide con el del original, comprobado sobre un archivo de 15 GB |
| RT-12 | S | E | Lectura por rango: solo se descargan los chunks que cubren el rango solicitado | `read(offset, len)` dentro de un único chunk genera exactamente una petición a un DataNode |
| RT-13 | S | D | El cliente verifica el checksum de cada chunk antes de reensamblar | Un chunk corrupto se detecta antes de escribir el archivo de salida y se reintenta contra otra réplica |

### Primitivas de acceso

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RT-14 | M | E | `open` abre sesión lógica: valida permisos y entrega manifiesto y tokens | El manifiesto se emite en tiempo de ejecución y tiene vigencia limitada |
| RT-15 | M | E | `close` ejecuta el commit y libera el lease | |
| RT-16 | M | E | `read(offset, len)` sobre rangos arbitrarios, incluidos los que cruzan fronteras de chunk | Un rango que abarca dos chunks descarga exactamente esos dos |
| RT-17 | M | E | `write` implementado como carga multiparte de una versión nueva | Una escritura en offset arbitrario sobre una versión existente se rechaza con error explícito |
| RT-18 | M | E | `lock` implementado como lease con expiración sobre la clave del objeto | Si el cliente titular desaparece, el lease vence y el objeto vuelve a estar disponible |

## 5. Requisitos del ControlNode

### Metadatos y namespace

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RN-01 | M | D | Árbol de namespace con directorios como entidades de primera clase, con dueño, grupo, permisos y ACL propios | `mkdir` de un directorio vacío crea una entidad persistente con permisos asignables |
| RN-02 | M | D | Mapa objeto a versión vigente, y versión a lista ordenada de chunks | El orden de la lista determina el reensamblado; alterarlo corrompe el archivo |
| RN-03 | M | D | Mapa chunk a ubicaciones mantenido en memoria y reconstruido desde los inventarios reportados | Tras reiniciar el ControlNode, las ubicaciones se repueblan sin leer disco propio |
| RN-04 | M | D | Write-ahead log con `fsync` antes de confirmar al cliente, más snapshot periódico | Terminación abrupta del proceso a mitad de operaciones; al reiniciar, el namespace está íntegro |
| RN-05 | M | D | Modo seguro al arrancar: atiende lecturas de metadatos pero no ordena re-replicación hasta recibir inventarios | Un reinicio del maestro no dispara copias masivas de chunks |
| RN-06 | S | D | Versionado inmutable: cada escritura crea una versión nueva y la anterior permanece hasta su recolección | Una escritura fallida deja el objeto anterior accesible e intacto |
| RN-07 | S | D | Recolector de chunks huérfanos que compara inventarios contra chunks referenciados por versiones vivas | Los chunks de una carga abortada desaparecen del disco tras un ciclo de recolección |

### Colocación

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RN-08 | M | E | Invariante dura: dos réplicas del mismo chunk nunca residen en el mismo DataNode | Verificable con `stat` sobre cualquier objeto |
| RN-09 | S | D | Invariante blanda: chunks consecutivos de un objeto se colocan preferentemente en nodos distintos | La lectura paraleliza sobre todos los nodos disponibles |
| RN-10 | M | P | Heurística de colocación con función objetivo explícita: minimizar la varianza de ocupación entre nodos | La asignación no es aleatoria pura ni round-robin ciego |
| RN-11 | M | P | Algoritmo de dos candidatos aleatorios con puntuación ponderada por ocupación, carga y dispersión | Tras escribir 100 chunks con clientes concurrentes, la diferencia de ocupación entre nodos permanece bajo el 10% |

### Membresía y recuperación

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RN-12 | M | D | Heartbeat periódico iniciado por el DataNode; el ControlNode nunca abre conexión hacia él | Las órdenes viajan en la respuesta al heartbeat |
| RN-13 | M | D | Clasificación en tres estados: vivo, sospechoso y muerto, con umbrales diferenciados | Un nodo que reinicia rápido no dispara re-replicación |
| RN-14 | M | E | Re-replicación automática de chunks sub-replicados, priorizando los que quedan con una sola copia | Al detener un DataNode, el sistema restaura R réplicas sin intervención |
| RN-15 | S | D | Detección y limpieza de sobre-replicación | Un nodo que reaparece tras una partición no deja copias excedentes permanentes |
| RN-16 | S | D | Control de ritmo: traslados concurrentes acotados y prioridad inferior del rebalanceo frente a la re-replicación | Con chunks sub-replicados pendientes, el rebalanceo queda suspendido |

### Elasticidad

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RN-17 | M | P | Rebalanceo activo al ingresar nodos, disparado por diferencia de ocupación superior al umbral | Agregar un nodo vacío a un clúster desbalanceado redistribuye chunks hacia él |
| RN-18 | M | D | Traslado por copia y verificación antes de eliminar el origen | Durante el traslado el chunk está sobre-replicado, nunca sub-replicado |
| RN-19 | S | P | Baja planificada con drenado: el nodo deja de recibir escrituras y sus chunks se evacúan antes de retirarlo | Al drenar, ningún chunk queda por debajo de R en ningún instante |
| RN-20 | M | P | Replicación efectiva degradada cuando el número de nodos es menor que R | Con 2 nodos y R=3 el sistema acepta escrituras y las marca como degradadas |
| RN-21 | M | P | Cola de reparación persistente que eleva los chunks degradados al recuperar nodos | Al volver a subir nodos, los chunks degradados alcanzan R sin intervención |
| RN-22 | M | P | Piso duro: con un solo DataNode disponible se rechazan las escrituras nuevas | El rechazo indica explícitamente la causa |
| RN-23 | M | P | Factor de replicación reconfigurable en caliente | Cambiar R de 3 a 4 programa las copias faltantes sin reiniciar el clúster |

### Concurrencia y alta disponibilidad

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RN-24 | M | D | Lease con expiración por clave de objeto, renovable durante cargas largas | Un cliente que desaparece libera el objeto al vencer el lease |
| RN-25 | S | D | Locks de lectura y escritura sobre el camino del namespace | Eliminar un directorio mientras se crean hijos no produce estado inconsistente |
| RN-26 | S | E | Plano de control replicado con elección de líder | La caída del líder no produce pérdida de metadatos confirmados |
| RN-27 | S | D | Imposibilidad de split-brain garantizada por quórum de mayoría | Un líder aislado no puede confirmar escrituras de metadatos |

## 6. Requisitos del DataNode

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RD-01 | M | E | Almacén local de chunks identificados globalmente, sin conocimiento del namespace | El DataNode no puede reconstruir a qué archivo pertenece un chunk |
| RD-02 | M | D | Checksum calculado en la escritura y verificado en cada lectura | Un chunk alterado en disco se detecta al servirlo y se reporta como corrupto |
| RD-03 | S | D | Barrido periódico de integridad sobre los chunks almacenados | Corromper un chunk con una escritura directa al volumen dispara re-replicación sin intervención |
| RD-04 | M | D | Motor de replicación en pipeline: recibe un chunk, lo escribe y lo reenvía al siguiente de la cadena | El ack se propaga en sentido inverso hasta el cliente |
| RD-05 | M | D | Agente de heartbeat que reporta estado, capacidad, carga e inventario de chunks | El ControlNode reconstruye el mapa de ubicaciones exclusivamente con estos reportes |
| RD-06 | M | D | Ejecución de órdenes recibidas en la respuesta al heartbeat: copiar chunk, eliminar chunk, reportar inventario | El DataNode nunca recibe conexiones entrantes iniciadas por el ControlNode |
| RD-07 | M | D | Validación local del token de acceso a chunk antes de servir o aceptar datos | Un token con firma inválida o vencido se rechaza sin consultar al ControlNode |
| RD-08 | S | E | Cifrado de cada chunk en reposo | El contenido del chunk en disco no es legible sin la clave correspondiente |
| RD-09 | S | D | Soporte de lectura parcial mediante cabecera de rango | Permite servir un fragmento sin transferir el chunk completo |
| RD-10 | S | D | Endpoint de estado con espacio libre, carga y número de chunks | Alimenta la función de puntuación de colocación del ControlNode |
| RD-11 | S | D | Estado de drenado que rechaza escrituras nuevas mientras evacúa sus chunks | Un nodo en drenado no aparece como destino en ninguna asignación |
| RD-12 | S | D | Endpoint anunciable configurable, distinto de la dirección de la red interna | Un cliente externo alcanza el DataNode aunque la dirección interna sea privada |

**Nota sobre RD-01.** La ausencia deliberada de conocimiento del namespace en el DataNode es lo que hace necesario el token de acceso a chunk: sin él, el nodo no tendría forma de decidir si un solicitante está autorizado.

## 7. Requisitos no funcionales

Cada uno enunciado con una métrica observable. Un requisito no funcional sin métrica no es verificable y no se incluye.

### Escalabilidad

| Id | P | F | Requisito | Métrica |
| --- | --- | --- | --- | --- |
| RQ-01 | M | E | Soporte de múltiples usuarios concurrentes | 20 clientes simultáneos sin errores atribuibles a contención |
| RQ-02 | M | E | Soporte de archivos de cualquier tamaño, limitado solo por la capacidad agregada | Transferencia verificada de un archivo de 15 GB |
| RQ-03 | S | D | Validación de identidad sin consulta a base de datos por petición | El token es autoverificable mediante firma |
| RQ-04 | S | D | El límite de escalabilidad es la memoria de metadatos del ControlNode, y debe estar cuantificado | Consumo declarado por objeto y por chunk |

### Disponibilidad

| Id | P | F | Requisito | Métrica |
| --- | --- | --- | --- | --- |
| RQ-05 | M | E | Tolerancia a la pérdida de R−1 DataNodes sin pérdida de datos | Con R=3, detener 2 nodos no impide recuperar ningún archivo |
| RQ-06 | M | E | Restauración automática del factor de replicación tras una caída | El sistema vuelve a R réplicas sin intervención manual |
| RQ-07 | S | D | Continuidad del servicio ante caída del nodo líder de control | Indisponibilidad de escritura inferior a 10 s |
| RQ-08 | S | D | Las lecturas en curso no se interrumpen por la caída del plano de control | Un `get` iniciado antes de la caída se completa |

### Consistencia

| Id | P | F | Requisito | Métrica |
| --- | --- | --- | --- | --- |
| RQ-09 | M | E | Lectura posterior a escritura confirmada devuelve siempre la versión nueva | Verificable desde un cliente distinto al que escribió |
| RQ-10 | M | D | Atomicidad de la publicación de versión | Ninguna lectura observa una versión parcialmente escrita |
| RQ-11 | M | D | Escrituras concurrentes sobre la misma clave se resuelven por exclusión, no por fusión | El segundo escritor recibe conflicto explícito |

### Rendimiento

| Id | P | F | Requisito | Métrica |
| --- | --- | --- | --- | --- |
| RQ-12 | M | E | Concurrencia en el acceso a archivos distintos y a datos dentro del mismo archivo | Varios clientes leen chunks distintos del mismo objeto simultáneamente |
| RQ-13 | M | D | Los datos no atraviesan el ControlNode en ningún flujo | El tráfico medido en el plano de control es proporcional al número de chunks, no a su tamaño |
| RQ-14 | S | D | El rendimiento agregado de lectura crece al agregar DataNodes | Tendencia creciente medida con 2, 4 y 6 nodos |

### Seguridad

| Id | P | F | Requisito | Métrica |
| --- | --- | --- | --- | --- |
| RQ-15 | M | E | Autenticación obligatoria en toda operación | Ninguna operación de namespace o de datos procede sin identidad verificada |
| RQ-16 | M | E | Contraseñas almacenadas con función de derivación resistente y salt por usuario | Nunca en texto plano ni con hash simple |
| RQ-17 | M | E | Control de acceso por usuario y grupo sobre objetos y directorios | Un usuario no lista ni accede a objetos ajenos sin permiso explícito |
| RQ-18 | M | E | Autorización en el plano de datos mediante token firmado y de vigencia corta | Un token falsificado o vencido es rechazado por el DataNode |
| RQ-19 | M | E | Cifrado en tránsito en todos los enlaces | Ningún enlace transporta datos ni credenciales en claro |
| RQ-20 | M | E | Autenticación mutua entre nodos de datos y de control | Un nodo sin certificado válido no se incorpora al clúster |
| RQ-21 | S | E | Cifrado de datos en reposo | El contenido en disco es ilegible sin la clave |
| RQ-22 | C | E | Segundo factor de autenticación y claves de API con ámbito | |
| RQ-23 | S | D | Registro de auditoría con identidad, operación, recurso, resultado y marca de tiempo | Permite reconstruir quién hizo qué y cuándo |

### Transparencia

| Id | P | F | Requisito | Métrica |
| --- | --- | --- | --- | --- |
| RQ-24 | M | E | Transparencia de localización: la ruta no revela ubicación física | Ninguna ruta del namespace codifica identidad de nodo |
| RQ-25 | M | E | Descubrimiento dinámico, sin configuración estática de localización | El cliente opera tras cambiar todas las direcciones de DataNodes, sin modificar su configuración |
| RQ-26 | S | D | Transparencia de fallos: la caída de una réplica no interrumpe la operación del cliente | El reintento contra otra réplica es invisible para el usuario |
| RQ-27 | C | E | Montaje del namespace como sistema de archivos local | Extensión opcional |

## 8. Requisitos de interfaz y protocolos

| Id | P | F | Requisito | Criterio de aceptación |
| --- | --- | --- | --- | --- |
| RI-01 | M | P | Contrato completo de la API del cliente: por cada operación, método, ruta, parámetros de entrada, estructura de respuesta y códigos de error | Un desarrollador ajeno al equipo implementa un cliente alterno usando solo el contrato |
| RI-02 | M | P | Toda comunicación entre nodos usa RPC con esquema tipado | Ningún enlace interno emplea REST |
| RI-03 | M | E | Los cinco enlaces del sistema están especificados: cliente a control, cliente a datos, control a control, control a datos, datos a datos | Cada enlace declara protocolo, dirección de iniciación y mecanismo de seguridad |
| RI-04 | M | D | Transferencia de chunks por flujo, sin cargar el chunk completo en memoria | El consumo de memoria del cliente no crece con el tamaño del archivo |
| RI-05 | S | D | Catálogo uniforme de errores con indicación de si la operación es reintentable | Los errores de conflicto se distinguen de los transitorios |
| RI-06 | S | D | Versionado del contrato de la API mediante prefijo de ruta | |
| RI-07 | S | D | El manifiesto de lectura se emite bajo demanda y caduca | Un manifiesto vencido obliga a solicitar uno nuevo |
| RI-08 | S | D | Endpoint de métricas por nodo | Expone latencia, rendimiento, chunks sub-replicados y nodos vivos |

### Enlaces del sistema

| Enlace | Protocolo | Quién inicia | Seguridad |
| --- | --- | --- | --- |
| Cliente a ControlNode | REST sobre HTTP | Cliente | Cifrado en tránsito y token de sesión |
| Cliente a DataNode | HTTP con flujo | Cliente | Cifrado en tránsito y token de chunk |
| ControlNode a ControlNode | RPC del protocolo de consenso | Cualquiera | Autenticación mutua |
| ControlNode a DataNode | RPC sobre heartbeat | **Siempre el DataNode** | Autenticación mutua |
| DataNode a DataNode | RPC con flujo | El anterior de la cadena | Autenticación mutua |

La columna *quién inicia* no es un detalle de implementación: determina la configuración de red, la tolerancia a NAT y el comportamiento del maestro ante nodos caídos.

## 9. Trazabilidad

Cada exigencia externa se conecta con la decisión de diseño que la resuelve y con los requisitos que la implementan. Sirve para verificar que ninguna quedó sin cubrir.

| Exigencia | Decisión de diseño | Requisitos |
| --- | --- | --- |
| Escritura y lectura distribuidas (RX-11) | Particionamiento interno en chunks; plano de datos separado | RT-01, RT-04, RT-10, RQ-13 |
| Cada archivo repartido entre nodos (RX-12) | Chunks de 64 MB con invariantes de colocación | RT-01, RT-04, RN-08, RN-09 |
| Gestión tipo Linux (RF1 del enunciado) | Namespace jerárquico sobre almacén de objetos | RC-02 a RC-07, RN-01 |
| Transferencia con send y receive (RF2) | `put` y `get` construidos sobre las primitivas | RT-01 a RT-13 |
| Primitivas de acceso (RF3) | Traducción a semántica de objetos: manifiesto, rango, multiparte, lease | RT-14 a RT-18 |
| Escalabilidad (RNF1) | Plano de datos horizontal; metadatos acotados | RQ-01 a RQ-04 |
| Alta disponibilidad (RNF2) | Replicación con factor parametrizable; plano de control replicado | RN-14, RN-26, RQ-05 a RQ-08 |
| Consistencia (RNF3) | Inmutabilidad y versionado; commit atómico como punto de serialización | RN-06, RT-07, RQ-09 a RQ-11 |
| Particionamiento (RNF4) | Chunks con invariantes duras y blandas | RN-08, RN-09, RT-04 |
| Rendimiento (RNF5) | Datos fuera del plano de control; descargas paralelas | RT-10, RQ-12 a RQ-14 |
| Seguridad (RNF6) | Identidad, ACL, tokens de chunk, cifrado en tránsito y reposo | RQ-15 a RQ-23, RD-07, RD-08 |
| Transparencia (RNF7) | Manifiesto dinámico; ninguna configuración estática | RC-09, RQ-24 a RQ-26 |
| Heurística de balanceo (indicación en clase) | Dos candidatos aleatorios con puntuación | RN-10, RN-11 |
| Rebalanceo al crecer el clúster (clase) | Traslado por copia y verificación, con umbrales | RN-17, RN-18 |
| Encogimiento del clúster (clase) | Drenado y replicación efectiva degradada | RN-19 a RN-23 |
| Factor de replicación parametrizable (RX-07) | Parámetro de clúster reconfigurable en caliente | RN-23 |
| Contrato de API real (RX-05) | Especificación completa de métodos y atributos | RI-01, RI-03 |
| RPC obligatorio entre nodos (RX-03) | RPC tipado en los cuatro enlaces internos | RI-02, RI-03 |
| Logs de particionamiento visibles (clase) | Instrumentación en cliente y en servicio | RC-08 |
| Vista propia por usuario (RX-13) | ACL sobre objetos y directorios | RQ-17, RN-01 |
| Integridad del archivo recuperado (clase) | Checksum por chunk y hash del objeto completo | RT-11, RT-13, RD-02 |

## 10. Supuestos, exclusiones y riesgos

### Supuestos

| Id | Supuesto | Si no se cumple |
| --- | --- | --- |
| SUP-01 | Todos los nodos pertenecen a la misma administración y confían entre sí bajo autenticación mutua | El modelo de confianza cambia y el cifrado del lado del cliente pasaría a ser obligatorio |
| SUP-02 | Los relojes de los nodos están sincronizados dentro de un margen de segundos | La expiración de leases y de tokens se vuelve imprecisa |
| SUP-03 | Los DataNodes disponen de almacenamiento local persistente entre reinicios | El inventario reportado sería siempre vacío y el sistema re-replicaría en exceso |
| SUP-04 | La red entre nodos del servicio es de baja latencia | Los umbrales de heartbeat y el ritmo de replicación requieren recalibración |
| SUP-05 | El cliente dispone de espacio local suficiente para el archivo completo en `get` | Sería necesario escribir por flujo directamente al destino |

### Exclusiones explícitas

Declaradas fuera de alcance por decisión de diseño. No son omisiones.

| Excluido | Razón |
| --- | --- |
| Actualización parcial in-place de un objeto | Contradice la semántica de escritura única adoptada |
| Operación de anexado a un objeto existente | Se modela como una versión nueva |
| Transacciones que abarquen varios objetos | Requeriría confirmación en dos fases sobre el namespace |
| Instantánea consistente de un directorio | No hay punto de serialización a nivel de directorio |
| Renombrado atómico de directorios con gran número de descendientes | El costo es proporcional al número de hijos |
| Enlaces duros y simbólicos | No contribuyen a ningún requisito del enunciado |
| Cumplimiento POSIX completo | Excluido por el modelo de objetos |
| Protección frente a un operador malicioso del propio servicio | Exigiría cifrado del lado del cliente, declarado como extensión |

### Riesgos

| Riesgo | Impacto | Mitigación |
| --- | --- | --- |
| La integración del consenso resulta más compleja de lo previsto | Compromete RN-26, RN-27 y RQ-07 | Usar librería madura; alternativa activo-pasivo declarada como limitación conocida |
| El cifrado en tránsito se introduce antes de estabilizar la lógica distribuida | Consume tiempo en depuración de certificados en lugar de protocolos | Introducirlo al final, con generación de certificados automatizada |
| Direcciones internas devueltas a clientes externos | La transferencia falla pese a estar el sistema sano | RD-12: endpoint anunciable explícito |
| Tormenta de re-replicación tras reiniciar el plano de control | Saturación de red y degradación del servicio | RN-05: modo seguro hasta recibir inventarios |
| Concentración de escrituras por heurística ingenua | Desequilibrio de ocupación y pérdida de paralelismo | RN-11: dos candidatos aleatorios con puntuación |
| Chunks huérfanos acumulados por cargas abortadas | Desperdicio permanente de almacenamiento | RN-07: recolección basada en inventarios |
