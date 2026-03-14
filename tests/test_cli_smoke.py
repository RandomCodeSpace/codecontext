import io
import zipfile
from http.server import HTTPServer, SimpleHTTPRequestHandler
from pathlib import Path
from threading import Thread

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


def test_from_url_downloads_and_extracts_db(tmp_path: Path, capsys):
    """Serve a zip containing a .db file via a local HTTP server and verify
    that -from-url downloads and extracts it."""

    # Create a small zip containing a dummy .db file.
    dummy_db = b"SQLite format 3\x00" + b"\x00" * 84  # minimal header
    zip_buf = io.BytesIO()
    with zipfile.ZipFile(zip_buf, "w") as zf:
        zf.writestr("baseline.db", dummy_db)
    zip_bytes = zip_buf.getvalue()

    # Spin up a tiny HTTP server that serves the zip.
    class Handler(SimpleHTTPRequestHandler):
        def do_GET(self):  # noqa: N802
            self.send_response(200)
            self.send_header("Content-Type", "application/zip")
            self.end_headers()
            self.wfile.write(zip_bytes)

        def log_message(self, *_args):
            pass  # silence server logs

    server = HTTPServer(("127.0.0.1", 0), Handler)
    port = server.server_address[1]
    thread = Thread(target=server.handle_request, daemon=True)
    thread.start()

    db_dest = tmp_path / "downloaded.db"
    code = run(["-graph", str(db_dest), "-from-url", f"http://127.0.0.1:{port}/db.zip", "stats"])
    thread.join(timeout=5)
    server.server_close()

    # The .db file should have been extracted to the destination path.
    assert db_dest.exists()
    assert db_dest.read_bytes()[:16] == b"SQLite format 3\x00"


def test_from_url_skips_when_db_exists(tmp_path: Path, capsys):
    """When the db already exists, -from-url should skip downloading."""
    db_path = tmp_path / "existing.db"
    db_path.write_text("existing", encoding="utf-8")

    code = run(["-graph", str(db_path), "-from-url", "http://example.com/fake.zip", "stats"])
    captured = capsys.readouterr()
    assert "skipping download" in captured.err
