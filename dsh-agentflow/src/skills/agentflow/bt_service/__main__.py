"""Entry point — runs the BT service JSON-RPC stdio loop.

Usage:
    python -m bt_service
    python -m bt_service --trees-dir /path/to/trees
"""

from __future__ import annotations
import argparse
import sys

from bt_service.server.handler import BTServer
from bt_service.server.transport import run_stdio_loop


def main():
    parser = argparse.ArgumentParser(description="agentflow BT service")
    parser.add_argument("--trees-dir", default="", help="Path to trees/ directory")
    args = parser.parse_args()

    server = BTServer(trees_dir=args.trees_dir)
    run_stdio_loop(server.handle)


if __name__ == "__main__":
    main()
