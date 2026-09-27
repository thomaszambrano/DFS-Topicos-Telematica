"""Consola interactiva de DFSha (bloque D.4).

La interfaz principal del cliente es de consola, como exige RX-08.

EL DIRECTORIO DE TRABAJO ES ESTADO DEL CLIENTE, no del servidor (RC-03). El
ControlNode no guarda ningún `cwd` por sesión: cada petición lleva su ruta absoluta,
ya resuelta aquí. Esa decisión es lo que permite que veinte clientes concurrentes no
interfieran entre sí y que el plano de control no tenga que recordar nada por
conexión (RQ-01).
"""

from __future__ import annotations

import posixpath
import shlex
import sys
import traceback
from datetime import datetime

from .api import ConflictError, ControlClient, DFShaError
from .transfer import _human, get, put


class Shell:
    def __init__(self, control: ControlClient):
        self.control = control
        self.cwd = "/"
        self.running = True

    # -- resolución de rutas, del lado del cliente --------------------------

    def resolve(self, path: str | None) -> str:
        """Convierte una ruta del usuario en una ruta absoluta normalizada.

        Acepta absolutas, relativas, "." y "..". posixpath.normpath resuelve los
        ".." de forma textual, y el join con "/" garantiza que nunca se pueda subir
        por encima de la raíz del namespace.
        """
        if not path:
            return self.cwd
        base = path if path.startswith("/") else posixpath.join(self.cwd, path)
        resolved = posixpath.normpath(base)
        return resolved if resolved.startswith("/") else "/" + resolved

    # -- comandos -----------------------------------------------------------

    def cmd_ls(self, args: list[str]) -> None:
        path = self.resolve(args[0] if args else None)
        data = self.control.list(path)
        entries = data["entries"]
        if not entries:
            return
        print(f"{'tipo':<7} {'permisos':<9} {'dueño':<12} {'tamaño':>11} {'chunks':>7}  modificado           nombre")
        for e in entries:
            kind = "dir" if e["kind"] == "dir" else "obj"
            mode = format(e["mode"], "04o")
            size = _human(e["size"]) if e["kind"] == "object" else "-"
            chunks = str(e["chunks"]) if e["chunks"] else "-"
            stamp = _stamp(e["modified_at"])
            name = e["name"] + ("/" if e["kind"] == "dir" else "")
            print(f"{kind:<7} {mode:<9} {e['owner']:<12} {size:>11} {chunks:>7}  {stamp}  {name}")

    def cmd_cd(self, args: list[str]) -> None:
        target = self.resolve(args[0] if args else "/")
        entry = self.control.stat(target)["entry"]
        if entry["kind"] != "dir":
            raise DFShaError(400, f"{target} no es un directorio")
        self.cwd = target

    def cmd_pwd(self, args: list[str]) -> None:
        print(self.cwd)

    def cmd_mkdir(self, args: list[str]) -> None:
        if not args:
            raise DFShaError(400, "uso: mkdir <ruta>")
        for raw in args:
            path = self.resolve(raw)
            self.control.mkdir(path)
            print(f"creado {path}")

    def cmd_rmdir(self, args: list[str]) -> None:
        if not args:
            raise DFShaError(400, "uso: rmdir <ruta>")
        for raw in args:
            path = self.resolve(raw)
            entry = self.control.stat(path)["entry"]
            if entry["kind"] != "dir":
                raise DFShaError(400, f"{path} no es un directorio; usa rm")
            self.control.remove(path)
            print(f"eliminado {path}")

    def cmd_rm(self, args: list[str]) -> None:
        if not args:
            raise DFShaError(400, "uso: rm <ruta>")
        for raw in args:
            path = self.resolve(raw)
            result = self.control.remove(path)
            print(f"eliminado {path} ({result['orphan_chunks']} chunks marcados para recolección)")

    def cmd_stat(self, args: list[str]) -> None:
        """Muestra tamaño, versión vigente y las réplicas REALES de cada chunk (RC-06).

        Es la forma de verificar en vivo cómo quedó distribuido un archivo entre los
        DataNodes, que es la evidencia central de RX-11 y RX-12.
        """
        if not args:
            raise DFShaError(400, "uso: stat <ruta>")
        path = self.resolve(args[0])
        data = self.control.stat(path)
        e = data["entry"]
        print(f"ruta        : {data['path']}")
        print(f"tipo        : {e['kind']}")
        print(f"dueño       : {e['owner']}   permisos: {format(e['mode'], '04o')}")
        print(f"creado      : {_stamp(e['created_at'])}")
        print(f"modificado  : {_stamp(e['modified_at'])}")
        if e["kind"] == "object":
            print(f"tamaño      : {_human(e['size'])} ({e['size']} bytes)")
            print(f"version_id  : {e.get('version_id', '-')}")
            print(f"sha256      : {e.get('sha256', '-')}")
            print(f"chunks      : {e['chunks']}")
        for c in data.get("placement", []):
            replicas = ", ".join(c["replicas"]) or "SIN RÉPLICAS VIVAS"
            print(f"  [{c['index']:>4}] {c['chunk_id']} | {_human(c['size']):>9} | {replicas}")

    def cmd_mv(self, args: list[str]) -> None:
        if len(args) != 2:
            raise DFShaError(400, "uso: mv <origen> <destino>")
        src, dst = self.resolve(args[0]), self.resolve(args[1])
        self.control.rename(src, dst)
        print(f"{src} -> {dst}")

    def cmd_put(self, args: list[str]) -> None:
        if not args:
            raise DFShaError(400, "uso: put <archivo-local> [ruta-remota]")
        local = args[0]
        remote = self.resolve(args[1] if len(args) > 1 else posixpath.basename(local))
        put(self.control, local, remote)

    def cmd_get(self, args: list[str]) -> None:
        if not args:
            raise DFShaError(400, "uso: get <ruta-remota> [archivo-local]")
        remote = self.resolve(args[0])
        local = args[1] if len(args) > 1 else posixpath.basename(remote)
        get(self.control, remote, local)

    def cmd_df(self, args: list[str]) -> None:
        d = self.control.df()
        print(f"capacidad   : {_human(d['capacity'])}")
        print(f"usado       : {_human(d['used'])}")
        print(f"libre       : {_human(d['free'])}")
        print(f"nodos vivos : {d['alive_nodes']}")
        print(f"chunks      : {d['known_chunks']}")
        print(f"chunk_size  : {_human(d['chunk_size'])}   R = {d['replication_factor']}")

    def cmd_nodes(self, args: list[str]) -> None:
        print(f"{'nodo':<14} {'estado':<9} {'endpoint':<26} {'usado':>10} {'capacidad':>10} {'chunks':>7}")
        for n in self.control.nodes()["nodes"]:
            print(f"{n['node_id']:<14} {n['status']:<9} {n['endpoint']:<26} "
                  f"{_human(n['used']):>10} {_human(n['capacity']):>10} {n['chunks']:>7}")

    def cmd_help(self, args: list[str]) -> None:
        print(HELP)

    def cmd_exit(self, args: list[str]) -> None:
        self.running = False

    COMMANDS = {
        "ls": cmd_ls, "cd": cmd_cd, "pwd": cmd_pwd, "mkdir": cmd_mkdir,
        "rmdir": cmd_rmdir, "rm": cmd_rm, "stat": cmd_stat, "mv": cmd_mv,
        "put": cmd_put, "get": cmd_get, "df": cmd_df, "nodes": cmd_nodes,
        "help": cmd_help, "?": cmd_help, "exit": cmd_exit, "quit": cmd_exit,
    }

    # -- bucle --------------------------------------------------------------

    def run_once(self, line: str) -> int:
        parts = shlex.split(line)
        if not parts:
            return 0
        name, args = parts[0], parts[1:]
        handler = self.COMMANDS.get(name)
        if handler is None:
            print(f"comando desconocido: {name}. Escribe 'help'.", file=sys.stderr)
            return 2
        try:
            handler(self, args)
            return 0
        except ConflictError as e:
            # 409 no se reintenta: otro cliente tiene la clave tomada (RQ-11).
            print(f"conflicto: {e.detail}", file=sys.stderr)
            return 1
        except DFShaError as e:
            print(f"error: {e.detail}", file=sys.stderr)
            return 1
        except FileNotFoundError as e:
            print(f"error: archivo local no encontrado: {e.filename}", file=sys.stderr)
            return 1
        except Exception:
            traceback.print_exc()
            return 1

    def repl(self) -> int:
        """Bucle interactivo. La sesión se abre una vez y emite muchos comandos (RC-01)."""
        print(f"DFSha shell — control: {self.control.base_url} — usuario: {self.control.user}")
        print("Escribe 'help' para ver los comandos, 'exit' para salir.\n")
        while self.running:
            try:
                line = input(f"dfsha:{self.cwd}$ ")
            except (EOFError, KeyboardInterrupt):
                print()
                break
            self.run_once(line)
        return 0


HELP = """comandos disponibles:
  ls [ruta]                lista un directorio: tipo, permisos, dueño, tamaño y fecha
  cd [ruta]                cambia de directorio (estado del cliente, no del servidor)
  pwd                      imprime el directorio actual
  mkdir <ruta>...          crea directorios (falla si el padre no existe)
  rmdir <ruta>...          elimina un directorio vacío
  rm <ruta>...             elimina un objeto
  stat <ruta>              detalle del objeto y las réplicas reales de cada chunk
  mv <origen> <destino>    renombra o mueve
  put <local> [remoto]     sube un archivo mostrando el reparto de chunks
  get <remoto> [local]     descarga en paralelo y verifica el sha256
  df                       ocupación agregada del clúster
  nodes                    estado de membresía de los DataNodes
  help                     esta ayuda
  exit                     salir"""


def _stamp(iso: str) -> str:
    try:
        return datetime.fromisoformat(iso.replace("Z", "+00:00")).strftime("%Y-%m-%d %H:%M:%S")
    except (ValueError, AttributeError):
        return str(iso)[:19]
