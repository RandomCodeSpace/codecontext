from __future__ import annotations

from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from codecontext.backends import open_backend
from codecontext.indexer import Indexer
from codecontext.web import create_app


def _make_app(tmp_path: Path, backend: str) -> tuple[TestClient, object]:
    project = tmp_path / "proj"
    project.mkdir()
    (project / "sample.py").write_text(
        "import os\n\nclass A:\n    def work(self):\n        return 1\n",
        encoding="utf-8",
    )

    db_path = tmp_path / f"graph-{backend}.db"
    db = open_backend(backend, str(db_path))
    idx = Indexer(db)
    idx.index_directory(str(project))
    app = create_app(idx)
    client = TestClient(app)
    return client, db


@pytest.mark.parametrize("backend", ["sqlite", "falkordblite"])
def test_web_endpoints(tmp_path: Path, backend: str):
    client, db = _make_app(tmp_path, backend)
    try:
        r = client.get("/")
        assert r.status_code == 200
        assert "<!DOCTYPE html>" in r.text
        assert "codecontext" in r.text

        r = client.get("/api/stats")
        assert r.status_code == 200
        body = r.json()
        assert body["files"] >= 1

        r = client.get("/api/graph")
        assert r.status_code == 200
        body = r.json()
        assert len(body["nodes"]) >= 1

        r = client.get("/api/tree")
        assert r.status_code == 200
        tree = r.json()
        assert tree["name"] == "."
        assert tree.get("lang") in {"", "python", "javascript", "java", "go", "typescript"}

        r = client.get("/api/dir", params={"path": ""})
        assert r.status_code == 200
        detail = r.json()
        assert "fileCount" in detail

        r = client.post("/mcp", json={"id": 1, "tool": "graph_stats", "arguments": {}})
        assert r.status_code == 200
        body = r.json()
        assert body["id"] == 1
        assert body["result"]["files"] >= 1
    finally:
        db.close()

@pytest.mark.parametrize("backend", ["sqlite", "falkordblite"])
def test_tree_directory_has_dominant_language(tmp_path: Path, backend: str):
    project = tmp_path / "proj"
    py_dir = project / "py"
    js_dir = project / "js"
    py_dir.mkdir(parents=True)
    js_dir.mkdir(parents=True)

    (py_dir / "a.py").write_text("def one():\n    return 1\n", encoding="utf-8")
    (py_dir / "b.py").write_text("def two():\n    return 2\n", encoding="utf-8")
    (js_dir / "x.js").write_text("function x() { return 1; }\n", encoding="utf-8")

    db_path = tmp_path / f"graph-{backend}.db"
    db = open_backend(backend, str(db_path))
    idx = Indexer(db)
    idx.index_directory(str(project))

    try:
        app = create_app(idx)
        client = TestClient(app)

        tree = client.get("/api/tree").json()

        def find_by_suffix(node: dict, suffix: str):
            if str(node.get("path", "")).endswith(suffix):
                return node
            for child in node.get("children", []):
                found = find_by_suffix(child, suffix)
                if found is not None:
                    return found
            return None

        py_node = find_by_suffix(tree, "/py")
        js_node = find_by_suffix(tree, "/js")
        assert py_node is not None
        assert js_node is not None
        assert py_node["lang"] == "python"
        assert js_node["lang"] == "javascript"
    finally:
        db.close()
