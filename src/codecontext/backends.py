from __future__ import annotations

from .db import Database
from .db_cogdb import CogDatabase
from .storage import StorageBackend

SUPPORTED_BACKENDS = {"sqlite", "cogdb"}


def open_backend(name: str, graph_db: str, verbose: bool = False) -> StorageBackend:
    backend = normalize_backend_name(name)
    if backend == "sqlite":
        return Database.open(graph_db, verbose)
    if backend == "cogdb":
        return CogDatabase.open(graph_db, verbose)
    raise RuntimeError(f"unsupported backend: {name}")


def normalize_backend_name(name: str) -> str:
    backend = (name or "sqlite").strip().lower()
    if backend not in SUPPORTED_BACKENDS:
        supported = ", ".join(sorted(SUPPORTED_BACKENDS))
        raise RuntimeError(f"unsupported backend '{name}'. supported backends: {supported}")
    return backend