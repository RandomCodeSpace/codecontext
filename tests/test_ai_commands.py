from __future__ import annotations

from pathlib import Path

from codecontext.cli import run


def _index_fixture(tmp_path: Path) -> tuple[Path, Path]:
    project = tmp_path / "proj"
    project.mkdir()
    sample = project / "sample.py"
    sample.write_text(
        "import os\n\nclass A:\n    def work(self):\n        return 1\n\ndef top():\n    return A()\n",
        encoding="utf-8",
    )
    db_path = tmp_path / "graph.db"
    assert run(["-graph", str(db_path), "index", str(project)]) == 0
    return db_path, sample


def test_ai_subcommands_with_mock_provider(tmp_path: Path, capsys, monkeypatch):
    db_path, sample = _index_fixture(tmp_path)
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    monkeypatch.setenv("LLM_MODEL", "mock-test")

    assert run(["-graph", str(db_path), "ai", "query", "what", "is", "top"]) == 0
    assert "[mock:mock-test]" in capsys.readouterr().out

    assert run(["-graph", str(db_path), "ai", "analyze", "top"]) == 0
    assert "[mock:mock-test]" in capsys.readouterr().out

    assert run(["-graph", str(db_path), "ai", "docs", "top"]) == 0
    assert "[mock:mock-test]" in capsys.readouterr().out

    assert run(["-graph", str(db_path), "ai", "review", "top"]) == 0
    assert "[mock:mock-test]" in capsys.readouterr().out

    assert run(["-graph", str(db_path), "ai", "summarize", str(sample)]) == 0
    assert "[mock:mock-test]" in capsys.readouterr().out


def test_ai_missing_subcommand_args(tmp_path: Path, capsys, monkeypatch):
    db_path, _sample = _index_fixture(tmp_path)
    monkeypatch.setenv("LLM_PROVIDER", "mock")

    assert run(["-graph", str(db_path), "ai", "docs"]) == 1
    err = capsys.readouterr().err
    assert "usage: codecontext ai docs" in err
