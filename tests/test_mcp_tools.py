from __future__ import annotations

from pathlib import Path

import pytest

from codecontext.backends import open_backend
from codecontext.indexer import Indexer
from codecontext.mcp import MCPService


def _build_service(tmp_path: Path, backend: str) -> tuple[MCPService, str, object]:
    project = tmp_path / "proj"
    project.mkdir()
    file_path = project / "sample.py"
    file_path.write_text(
        "import os\n\nclass A:\n    def work(self):\n        return 1\n\ndef top():\n    return A()\n",
        encoding="utf-8",
    )

    db_path = tmp_path / f"graph-{backend}.db"
    db = open_backend(backend, str(db_path))
    idx = Indexer(db)
    idx.index_directory(str(project))
    return MCPService(db, idx), str(file_path), db


@pytest.mark.parametrize("backend", ["sqlite", "falkordblite"])
def test_graph_stats_and_list_files(tmp_path: Path, backend: str):
    service, file_path, db = _build_service(tmp_path, backend)
    try:
        stats = service.call_tool("graph_stats")
        assert stats["files"] >= 1
        assert stats["entities"] >= 1

        listed = service.call_tool("list_files", {"language": "python"})
        assert listed["count"] >= 1
        assert any(row["path"] == file_path for row in listed["files"])
    finally:
        db.close()

@pytest.mark.parametrize("backend", ["sqlite", "falkordblite"])
def test_search_query_outline_imports_and_code(tmp_path: Path, backend: str):
    service, file_path, db = _build_service(tmp_path, backend)
    try:
        queried = service.call_tool("query_entity", {"name": "top"})
        assert queried["count"] >= 1
        entity_id = queried["results"][0]["id"]

        searched = service.call_tool("search_entities", {"query": "top", "limit": 10})
        assert searched["count"] >= 1

        outline = service.call_tool("get_file_outline", {"path": file_path})
        assert outline["count"] >= 1

        imports = service.call_tool("get_file_imports", {"path": file_path})
        assert imports["count"] >= 1
        assert "os" in imports["imports"]

        code = service.call_tool("get_entity_code", {"entity_id": entity_id})
        assert "def top" in code["code"]
    finally:
        db.close()

@pytest.mark.parametrize("backend", ["sqlite", "falkordblite"])
def test_find_usages_returns_shape(tmp_path: Path, backend: str):
    service, _file_path, db = _build_service(tmp_path, backend)
    try:
        queried = service.call_tool("query_entity", {"name": "work"})
        assert queried["count"] >= 1
        entity_id = queried["results"][0]["id"]

        usages = service.call_tool("find_usages", {"entity_id": entity_id})
        assert usages["entity_id"] == entity_id
        assert "callers" in usages
    finally:
        db.close()
