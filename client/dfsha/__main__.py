"""Punto de entrada de la CLI: `python -m dfsha`."""

from __future__ import annotations

import argparse
import getpass
import os
import sys

from .api import ControlClient, DFShaError
from .shell import Shell


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="dfsha",
        description="Cliente de DFSha. Solo necesita la dirección del ControlNode: "
                    "las direcciones de los DataNodes se descubren en tiempo de ejecución.",
    )
    parser.add_argument(
        "--control",
        default=os.getenv("DFSHA_CONTROL", "http://localhost:8080"),
        help="URL del ControlNode (o variable DFSHA_CONTROL)",
    )
    parser.add_argument(
        "--user",
        default=os.getenv("DFSHA_USER", os.getenv("USER", "anonymous")),
        help="usuario con el que se inicia sesión (o variable DFSHA_USER)",
    )
    parser.add_argument(
        "--register",
        action="store_true",
        help="crea el usuario antes de iniciar sesión (si ya existe, solo inicia sesión)",
    )

    sub = parser.add_subparsers(dest="command")
    sub.add_parser("shell", help="consola interactiva (por omisión)")
    run = sub.add_parser("run", help="ejecuta un comando y termina")
    run.add_argument("args", nargs=argparse.REMAINDER)

    opts = parser.parse_args(argv)
    control = ControlClient(opts.control, opts.user)
    # DFSHA_PASSWORD existe para los scripts (verificar.sh); a mano se pide sin eco.
    password = os.getenv("DFSHA_PASSWORD") or getpass.getpass(f"contraseña de {opts.user}: ")
    try:
        if opts.register:
            try:
                control.register(opts.user, password)
            except DFShaError as e:
                if e.status != 409:  # 409: ya existía, se sigue al login
                    raise
        home = control.login(opts.user, password)["home"]
    except DFShaError as e:
        print(f"no se pudo iniciar sesión: {e.detail}", file=sys.stderr)
        return 1

    shell = Shell(control)
    shell.cwd = home

    if opts.command == "run":
        return shell.run_once(" ".join(opts.args))
    return shell.repl()


if __name__ == "__main__":
    sys.exit(main())
