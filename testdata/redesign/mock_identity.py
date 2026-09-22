"""Synthetic OAuth userinfo provider. Only mounted on the R1 internal network.

No enterprise identity, credentials, external requests, or model calls are used.
This fixture deliberately does not claim to validate a production identity provider.
"""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, data):
        body = json.dumps(data).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        form = parse_qs(body.decode())
        self.reply({"access_token": form.get("code", ["employee"])[0], "token_type": "Bearer"})

    def do_GET(self):
        subject = self.headers.get("Authorization", "Bearer employee").removeprefix("Bearer ")
        self.reply({"sub": subject, "name": subject, "email": subject + "@example.invalid"})


HTTPServer(("0.0.0.0", 18000), Handler).serve_forever()
