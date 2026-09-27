"""SDK y CLI de DFSha.

Uso como biblioteca, sin pasar por la consola (RC-12):

    from dfsha import ControlClient, put, get

    control = ControlClient("http://localhost:8080", user="thomas")
    put(control, "video.mp4", "/media/video.mp4")
    get(control, "/media/video.mp4", "copia.mp4")
"""

from .api import ConflictError, ControlClient, DataClient, DFShaError
from .chunker import DEFAULT_CHUNK_SIZE, sha256_of_bytes, sha256_of_file, split_file
from .transfer import get, put

__all__ = [
    "ConflictError", "ControlClient", "DataClient", "DFShaError",
    "DEFAULT_CHUNK_SIZE", "sha256_of_bytes", "sha256_of_file", "split_file",
    "get", "put",
]
