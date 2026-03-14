from __future__ import annotations

from pathlib import Path

from codecontext.cli import run


def test_docs_stdout_and_file_output(tmp_path: Path, capsys):
    project = tmp_path / "proj"
    project.mkdir()
    (project / "sample.py").write_text(
        "class A:\n    def work(self):\n        return 1\n",
        encoding="utf-8",
    )

    db_path = tmp_path / "graph.db"
    assert run(["-graph", str(db_path), "index", str(project)]) == 0

    assert run(["-graph", str(db_path), "docs"]) == 0
    output = capsys.readouterr().out
    assert "# Project Documentation" in output
    assert "sample.py" in output
    assert "A.work" in output

    docs_file = tmp_path / "docs.md"
    assert run(["-graph", str(db_path), "docs", "-output", str(docs_file)]) == 0
    assert docs_file.exists()
    content = docs_file.read_text(encoding="utf-8")
    assert "# Project Documentation" in content


def test_docs_ai_with_mock_provider(tmp_path: Path, capsys, monkeypatch):
    project = tmp_path / "proj"
    project.mkdir()
    (project / "sample.py").write_text("def top():\n    return 1\n", encoding="utf-8")

    db_path = tmp_path / "graph.db"
    assert run(["-graph", str(db_path), "index", str(project)]) == 0

    monkeypatch.setenv("LLM_PROVIDER", "mock")
    monkeypatch.setenv("LLM_MODEL", "test-model")

    code = run(["-graph", str(db_path), "docs", "-ai"])
    assert code == 0
    out = capsys.readouterr().out
    assert "Project Documentation (AI-generated)" in out
