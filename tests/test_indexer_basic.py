from __future__ import annotations

from pathlib import Path

from codecontext.backends import open_backend
from codecontext.cli import run


def test_index_and_query_flow(tmp_path: Path, capsys):
    project = tmp_path / "proj"
    project.mkdir()
    (project / "sample.py").write_text(
        "import os\n\nclass A:\n    def work(self):\n        return 1\n\ndef top():\n    return A()\n",
        encoding="utf-8",
    )

    db_path = tmp_path / "graph.cog.db"

    code = run(["-graph", str(db_path), "index", str(project)])
    assert code == 0

    code = run(["-graph", str(db_path), "stats"])
    assert code == 0
    out = capsys.readouterr().out
    assert "Files:" in out
    assert "Entities:" in out

    code = run(["-graph", str(db_path), "query", "entity", "top"])
    assert code == 0
    out = capsys.readouterr().out
    assert "Found" in out
    assert "top" in out

    sqlite_db_path = tmp_path / "graph.sqlite.db"
    code = run(["-backend", "sqlite", "-graph", str(sqlite_db_path), "index", str(project)])
    assert code == 0
    code = run(["-backend", "sqlite", "-graph", str(sqlite_db_path), "stats"])
    assert code == 0
    out = capsys.readouterr().out
    assert "Files:" in out
    assert "Entities:" in out


def test_cogdb_backend_index_and_stats(tmp_path: Path, capsys):
    project = tmp_path / "proj"
    project.mkdir()
    (project / "sample.py").write_text("def top():\n    return 1\n", encoding="utf-8")

    db_path = tmp_path / "graph.cog.db"

    code = run(["-backend", "cogdb", "-graph", str(db_path), "index", str(project)])
    assert code == 0

    code = run(["-backend", "cogdb", "-graph", str(db_path), "stats"])
    assert code == 0
    out = capsys.readouterr().out
    assert "Files:" in out
    assert "Entities:" in out


def test_cogdb_parity_with_sqlite(tmp_path: Path):
    project = tmp_path / "proj"
    project.mkdir()
    (project / "a.py").write_text(
        "import os\n\nclass A:\n    def work(self):\n        return os.getcwd()\n",
        encoding="utf-8",
    )
    (project / "b.py").write_text(
        "from a import A\n\ndef top():\n    return A().work()\n",
        encoding="utf-8",
    )

    cog_path = tmp_path / "graph.cog.db"
    sqlite_path = tmp_path / "graph.sqlite.db"

    assert run(["-backend", "cogdb", "-graph", str(cog_path), "index", str(project)]) == 0
    assert run(["-backend", "sqlite", "-graph", str(sqlite_path), "index", str(project)]) == 0

    cog_db = open_backend("cogdb", str(cog_path))
    sqlite_db = open_backend("sqlite", str(sqlite_path))
    try:
        assert cog_db.get_file_count() == sqlite_db.get_file_count()
        assert cog_db.get_entity_count() == sqlite_db.get_entity_count()
        assert cog_db.get_dependency_count() == sqlite_db.get_dependency_count()
        assert cog_db.get_relation_count() == sqlite_db.get_relation_count()

        cog_names = sorted(entity.name for entity in cog_db.get_all_entities())
        sqlite_names = sorted(entity.name for entity in sqlite_db.get_all_entities())
        assert cog_names == sqlite_names
    finally:
        cog_db.close()
        sqlite_db.close()


def test_index_parallel_mode(tmp_path: Path, capsys):
    project = tmp_path / "proj"
    project.mkdir()
    (project / "a.py").write_text("def one():\n    return 1\n", encoding="utf-8")
    (project / "b.py").write_text("def two():\n    return 2\n", encoding="utf-8")
    (project / "c.js").write_text("function three(){ return 3 }\n", encoding="utf-8")

    db_path = tmp_path / "graph.parallel.db"

    code = run(["-graph", str(db_path), "index", str(project), "-parallel", "-jobs", "2"])
    assert code == 0

    code = run(["-graph", str(db_path), "stats"])
    assert code == 0
    out = capsys.readouterr().out
    assert "Files:" in out
    assert "Entities:" in out
