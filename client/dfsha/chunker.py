"""Particionamiento y verificación de integridad en el cliente (bloque D.1).

Este módulo no habla con la red: parte archivos y calcula hashes. Se prueba solo.
"""

from __future__ import annotations

import hashlib
import os
from typing import Iterator

# Tamaño por omisión, en bytes. El ControlNode es quien decide el tamaño real de
# cada carga y lo informa en la respuesta de open; este valor solo es el respaldo
# cuando se usa el chunker de forma aislada (RT-01).
DEFAULT_CHUNK_SIZE = 64 * 1024 * 1024

# Tamaño del bloque de lectura al calcular hashes. Ni el hash ni la subida cargan
# el archivo completo en memoria (RI-04).
READ_BLOCK = 1024 * 1024


def chunk_count(size: int, chunk_size: int) -> int:
    """Número de chunks que produce un archivo de `size` bytes.

    Es una división con techo: los 15 GB del criterio RT-01 dan 240 chunks de 64 MB,
    y un archivo de 1 byte da 1 chunk, no 0.
    """
    if size <= 0:
        return 0
    return (size + chunk_size - 1) // chunk_size


def split_file(path: str, chunk_size: int = DEFAULT_CHUNK_SIZE) -> Iterator[tuple[int, bytes]]:
    """Genera pares (índice, datos) leyendo el archivo por partes.

    EL ÚLTIMO CHUNK CONSERVA SU TAMAÑO REAL Y NO SE RELLENA (RT-02). Es el punto
    donde más gente se equivoca: rellenar con ceros hasta completar 64 MB hace que
    el archivo reconstruido sea más grande que el original y que su SHA-256 no
    coincida. Un ejecutable así deja de ejecutar, y un video deja de reproducirse,
    aunque todos los bytes "útiles" estén ahí.

    Es un generador y no una lista para que subir un archivo de 15 GB no requiera
    15 GB de RAM: cada chunk se lee, se sube y se descarta.
    """
    if chunk_size <= 0:
        raise ValueError(f"chunk_size debe ser positivo, recibí {chunk_size}")

    with open(path, "rb") as f:
        index = 0
        while True:
            data = f.read(chunk_size)
            if not data:
                # Fin del archivo. Un archivo vacío no produce ningún chunk.
                return
            yield index, data
            index += 1
            # read() devuelve menos que chunk_size solo al final del archivo, así
            # que un chunk corto significa que ya terminamos.
            if len(data) < chunk_size:
                return


def sha256_of_file(path: str) -> str:
    """SHA-256 del archivo completo, en hexadecimal.

    Hexadecimal porque es lo que imprime `sha256sum` en la terminal y lo que
    produce el servidor en Go: la comparación de la demostración es visual, sin
    conversiones de formato de por medio (ver D-003 en DECISIONES.md).
    """
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while block := f.read(READ_BLOCK):
            h.update(block)
    return h.hexdigest()


def sha256_of_bytes(data: bytes) -> str:
    """SHA-256 de un bloque de bytes, en hexadecimal."""
    return hashlib.sha256(data).hexdigest()


def file_size(path: str) -> int:
    """Tamaño del archivo en bytes."""
    return os.path.getsize(path)
