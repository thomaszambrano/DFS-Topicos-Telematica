"""Punto de entrada de la CLI: `python -m dfsha`."""

from __future__ import annotations

import argparse
import os
import sys

from .api import ControlClient
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
        help="identidad del solicitante (o variable DFSHA_USER)",
    )

    sub = parser.add_subparsers(dest="command")
    sub.add_parser("shell", help="consola interactiva (por omisión)")
    run = sub.add_parser("run", help="ejecuta un comando y termina")
    run.add_argument("args", nargs=argparse.REMAINDER)

    opts = parser.parse_args(argv)
    shell = Shell(ControlClient(opts.control, opts.user))

    if opts.command == "run":
        return shell.run_once(" ".join(opts.args))
    return shell.repl()


if __name__ == "__main__":
    sys.exit(main())
