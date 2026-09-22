"""Loopback-only static SPA server for R2 synthetic browser checks."""
import argparse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("directory")
parser.add_argument("--port", type=int, default=14821)
args = parser.parse_args()

class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *a, **kw):
        super().__init__(*a, directory=args.directory, **kw)
    def log_message(self, *_):
        pass
    def do_GET(self):
        if not Path(self.translate_path(self.path)).is_file():
            self.path = "/index.html"
        super().do_GET()

ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()
