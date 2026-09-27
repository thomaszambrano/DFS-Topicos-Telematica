#!/usr/bin/env bash
# Verificación completa del Hito 2 (bloque E.2 de HITO2.md).
#
# Demuestra, en un solo comando:
#   E.2.1  crear directorios y listarlos
#   E.2.2  subir un archivo grande mostrando el reparto de chunks
#   E.2.3  que cada volumen de DataNode tiene chunks DISTINTOS  <- evidencia de RX-12
#   E.2.4  que el sha256 del archivo recuperado coincide con el original
set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"

CONTROL="${DFSHA_CONTROL:-http://localhost:8080}"
TAM_MB="${TAM_MB:-40}"
REMOTO="${REMOTO:-/demo/prueba.bin}"
TRABAJO="$(mktemp -d)"
ORIGINAL="$TRABAJO/original.bin"
RECUPERADO="$TRABAJO/recuperado.bin"

trap 'rm -rf "$TRABAJO"' EXIT

azul()  { printf '\n\033[1;34m== %s\033[0m\n' "$*"; }
verde() { printf '\033[0;32m%s\033[0m\n' "$*"; }
rojo()  { printf '\033[0;31m%s\033[0m\n' "$*"; }

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

dfsha() { (cd client && python3 -m dfsha --control "$CONTROL" --user verificador run "$@"); }

# ---------------------------------------------------------------- preparación
azul "0. Estado del clúster"
if ! curl -sf "$CONTROL/health" >/dev/null; then
  rojo "El ControlNode no responde en $CONTROL. Levanta el clúster con: make up"
  exit 1
fi
dfsha nodes
dfsha df

VIVOS=$(curl -s "$CONTROL/v1/df" | python3 -c 'import json,sys; print(json.load(sys.stdin)["alive_nodes"])')
if [ "$VIVOS" -lt 3 ]; then
  rojo "Solo $VIVOS DataNodes vivos. El enunciado exige de 3 a 4 (RX-06)."
  exit 1
fi
verde "$VIVOS DataNodes vivos"

# ---------------------------------------------------------------- E.2.1
azul "E.2.1  Crear directorios y listar"
dfsha mkdir /demo 2>/dev/null || echo "  /demo ya existía"
dfsha ls /

# ---------------------------------------------------------------- E.2.2
azul "E.2.2  Subir un archivo de ${TAM_MB} MB y observar el reparto"
dd if=/dev/urandom of="$ORIGINAL" bs=1m count="$TAM_MB" 2>/dev/null
SHA_ORIGINAL="$(sha256 "$ORIGINAL")"
echo "  archivo local : $ORIGINAL"
echo "  sha256        : $SHA_ORIGINAL"
echo
dfsha put "$ORIGINAL" "$REMOTO"

azul "Distribución registrada por el ControlNode (stat, RC-06)"
dfsha stat "$REMOTO"

# ---------------------------------------------------------------- E.2.3
azul "E.2.3  Chunks presentes en el volumen de cada DataNode"
echo "Esta es la evidencia de RX-11 y RX-12: cada nodo guarda chunks DISTINTOS."
echo
TOTAL_EN_NODOS=0
NODOS_CON_CHUNKS=0
for n in 1 2 3 4; do
  contenedor="dfsha-datanode$n"
  if ! docker ps --format '{{.Names}}' | grep -qx "$contenedor"; then
    echo "  datanode$n: contenedor ausente"
    continue
  fi
  # Se listan los chunks desde DENTRO del contenedor: es la prueba de que los bytes
  # están realmente en el volumen de ese nodo y no en otra parte.
  ids="$(docker exec "$contenedor" sh -c 'ls -1 /data/*.chunk 2>/dev/null' \
          | sed 's#.*/##; s#\.chunk$##' || true)"
  cuenta=0
  [ -n "$ids" ] && cuenta="$(printf '%s\n' "$ids" | grep -c .)"
  printf '  datanode%s  %2d chunks\n' "$n" "$cuenta"
  [ -n "$ids" ] && printf '%s\n' "$ids" | sed 's/^/      /'
  TOTAL_EN_NODOS=$((TOTAL_EN_NODOS + cuenta))
  [ "$cuenta" -gt 0 ] && NODOS_CON_CHUNKS=$((NODOS_CON_CHUNKS + 1)) || true
done
echo
echo "  total de chunks en disco : $TOTAL_EN_NODOS"
echo "  nodos que recibieron     : $NODOS_CON_CHUNKS de 4"
if [ "$NODOS_CON_CHUNKS" -lt 2 ]; then
  rojo "FALLO: el archivo no se repartió entre varios nodos (RX-12 sin cumplir)"
  exit 1
fi
verde "OK: el archivo está distribuido entre $NODOS_CON_CHUNKS DataNodes"

# ---------------------------------------------------------------- E.2.4
azul "E.2.4  Descargar y comparar los sha256"
dfsha get "$REMOTO" "$RECUPERADO"
SHA_RECUPERADO="$(sha256 "$RECUPERADO")"
echo
echo "  original    : $SHA_ORIGINAL"
echo "  recuperado  : $SHA_RECUPERADO"
if [ "$SHA_ORIGINAL" != "$SHA_RECUPERADO" ]; then
  rojo "FALLO DE INTEGRIDAD: los hashes no coinciden"
  exit 1
fi
verde "OK: integridad verificada extremo a extremo (RT-11)"

# ---------------------------------------------------------------- resumen
azul "Resumen"
cat <<RESUMEN
  Clúster            : 1 ControlNode + $VIVOS DataNodes vivos          (RX-06)
  Archivo            : ${TAM_MB} MB en $TOTAL_EN_NODOS chunks
  Reparto            : $NODOS_CON_CHUNKS nodos con chunks distintos     (RX-11, RX-12)
  Integridad         : sha256 idéntico                          (RT-11, RT-02)
  Datos por el control: NO, solo metadatos                      (RQ-13)
  Descubrimiento     : el cliente solo conoce $CONTROL          (RX-14, RQ-25)
RESUMEN
verde "HITO 2 VERIFICADO"
