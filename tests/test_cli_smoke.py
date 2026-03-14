from codecontext.cli import run


def test_help_exits_zero(capsys):
    code = run(["-help"])
    assert code == 0
    captured = capsys.readouterr()
    assert "Usage:" in captured.err


def test_version_exits_zero(capsys):
    code = run(["-version"])
    assert code == 0
    captured = capsys.readouterr()
    assert "codecontext" in captured.out


def test_ai_requires_subcommand(capsys):
    code = run(["ai"])
    assert code == 1
    captured = capsys.readouterr()
    assert "usage: codecontext ai" in captured.err


def test_query_requires_args(capsys):
    code = run(["query", "entity"])
    assert code == 1
    captured = capsys.readouterr()
    assert "usage: codecontext query" in captured.err
