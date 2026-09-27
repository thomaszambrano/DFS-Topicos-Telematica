#!/usr/bin/env bash
# Levanta el clúster completo como procesos locales, sin Docker.
#
# RX-09 admite ejecución nativa o en contenedor. Esta variante sirve para
# desarrollar sin esperar builds de imágenes; la entrega oficial usa docker compose.
#
#   ./scripts/correr-nativo.sh iniciar    levanta 1 ControlNode + 4 DataNodes
#   ./scripts/correr-nativo.sh detener    los detiene
#   ./scripts/correr-nativo.sh estado     muestra qué está corriendo
set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"

BASE="${DFSHA_BASE:-$RAIZ/.run}"
CHUNK_SIZE="${CHUNK_SIZE:-4194304}"   # 4 MB para que el reparto se vea con archivos pequeños
NODOS="${NODOS:-4}"

iniciar() {
  mkdir -p "$BASE/log"
  go build -o "$BASE/controlnode" ./cmd/controlnode
  go build -o "$BASE/datanode" ./cmd/datanode

  HTTP_ADDR=":8080" GRPC_ADDR=":9090" CHUNK_SIZE="$CHUNK_SIZE" REPLICATION_FACTOR=1 \
    "$BASE/controlnode" >"$BASE/log/controlnode.log" 2>&1 &
  echo $! > "$BASE/controlnode.pid"

  for n in $(seq 1 "$NODOS"); do
    puerto=$((8080 + n))
    mkdir -p "$BASE/data/dn$n"
    NODE_ID="datanode$n" \
    DATA_DIR="$BASE/data/dn$n" \
    LISTEN_ADDR=":$puerto" \
    ADVERTISED_ENDPOINT="localhost:$puerto" \
    CONTROL_ADDR="localhost:9090" \
    CAPACITY="10737418240" \
      "$BASE/datanode" >"$BASE/log/datanode$n.log" 2>&1 &
    echo $! > "$BASE/datanode$n.pid"
  done

  # Esperar a que el primer heartbeat de cada nodo llegue al ControlNode.
  for _ in $(seq 1 30); do
    vivos=$(curl -s http://localhost:8080/v1/df 2>/dev/null \
            | python3 -c 'import json,sys; print(json.load(sys.stdin)["alive_nodes"])' 2>/dev/null || echo 0)
    [ "$vivos" = "$NODOS" ] && break
    sleep 0.5
  done
  echo "clúster nativo arriba: $vivos/$NODOS DataNodes vivos"
  echo "  datos en $BASE/data   logs en $BASE/log"
}

detener() {
  for f in "$BASE"/*.pid; do
    [ -f "$f" ] || continue
    kill "$(cat "$f")" 2>/dev/null || true
    rm -f "$f"
  done
  echo "clúster detenido"
}

estado() {
  curl -s http://localhost:8080/v1/nodes | python3 -m json.tool 2>/dev/null \
    || echo "el ControlNode no responde"
}

case "${1:-iniciar}" in
  iniciar) iniciar ;;
  detener) detener ;;
  estado)  estado ;;
  *) echo "uso: $0 {iniciar|detener|estado}"; exit 1 ;;
esac
