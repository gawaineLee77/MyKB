"""Synthetic model server with operation counts, never stores prompts/documents."""
import sys
import threading
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
sys.path.insert(0, "/fixture")
from mock_openai import Handler

counts = {}
lock = threading.Lock()


class Counted(Handler):
    def do_GET(self):
        if self.path == "/counts":
            with lock:
                self._json(200, dict(counts))
            return
        super().do_GET()

    def do_POST(self):
        with lock:
            counts[self.path] = counts.get(self.path, 0) + 1
        super().do_POST()

    def log_message(self, *args):
        pass


class NativeProxy(BaseHTTPRequestHandler):
    """Count execution endpoints while forwarding original credentials privately."""
    def log_message(self, *args): pass
    def relay(self):
        path = self.path.split("?", 1)[0]
        if self.command == "POST" and (path.startswith(("/api/v1/knowledge-search", "/api/v1/knowledge-chat/", "/api/v1/agent-chat/")) or path.endswith("/hybrid-search") or path == "/api/v1/messages/search"):
            with lock:
                # Count the endpoint family; no session IDs, queries or headers.
                family = "/".join(path.split("/")[:4]) if not path.endswith("/hybrid-search") else "hybrid-search"
                name = "native:" + family
                counts[name] = counts.get(name, 0) + 1
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        headers = {k:v for k,v in self.headers.items() if k.lower() not in {"host", "connection", "transfer-encoding"}}
        conn = http.client.HTTPConnection("app", 8080, timeout=90)
        try:
            conn.request(self.command, self.path, body=body or None, headers=headers)
            response = conn.getresponse()
            self.send_response(response.status)
            for k,v in response.getheaders():
                if k.lower() not in {"connection", "transfer-encoding"}: self.send_header(k,v)
            self.end_headers()
            if self.command != "HEAD":
                while chunk := response.read1(65536):
                    self.wfile.write(chunk)
                    self.wfile.flush()
        except (OSError, http.client.HTTPException):
            self.close_connection = True
        finally: conn.close()
    do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = do_HEAD = relay


threading.Thread(target=ThreadingHTTPServer(("0.0.0.0", 19091), NativeProxy).serve_forever, daemon=True).start()
ThreadingHTTPServer(("0.0.0.0", 19090), Counted).serve_forever()
