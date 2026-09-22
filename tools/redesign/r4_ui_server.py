"""Loopback SPA and credential-preserving proxy to a disposable R4 gateway."""
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import base64
import json
import subprocess

TRANSPORT = '''
import base64,http.client,json,sys
p=json.load(sys.stdin)
c=http.client.HTTPConnection('gateway',8080,timeout=90)
c.request(p['method'],p['path'],body=base64.b64decode(p['body']),headers=p['headers'])
r=c.getresponse()
print(json.dumps({'status':r.status,'headers':r.getheaders()}),flush=True)
while True:
 data=r.read1(8192)
 if not data: break
 print(json.dumps({'chunk':base64.b64encode(data).decode()}),flush=True)
c.close()
'''


def create_server(directory, compose_command):
    class Handler(SimpleHTTPRequestHandler):
        protocol_version = "HTTP/1.0"
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=str(directory() if callable(directory) else directory), **kwargs)
        def log_message(self, *_): pass
        def do_GET(self):
            if self.path.startswith(("/api/", "/mcp")): return self.proxy()
            if not Path(self.translate_path(self.path)).is_file(): self.path = "/index.html"
            return super().do_GET()
        def proxy(self):
            process = None
            try:
                body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                headers = {k: v for k, v in self.headers.items() if k.lower() not in ("host", "connection", "accept-encoding")}
                headers["Host"] = "gateway:8080"
                process = subprocess.Popen([*compose_command, 'exec', '-T', 'identity', 'python', '-c', TRANSPORT], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
                process.stdin.write(json.dumps({'method':self.command,'path':self.path,'body':base64.b64encode(body).decode(),'headers':headers}).encode()); process.stdin.close()
                response = json.loads(process.stdout.readline())
                self.send_response(response['status'])
                for key, value in response['headers']:
                    if key.lower() not in ("connection", "transfer-encoding", "content-length"): self.send_header(key, value)
                self.end_headers()
                for line in process.stdout:
                    self.wfile.write(base64.b64decode(json.loads(line)['chunk'])); self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError): pass
            finally:
                if process:
                    if process.poll() is None: process.terminate()
                    process.wait(timeout=5)
        do_POST = do_PUT = do_PATCH = do_DELETE = proxy
    return ThreadingHTTPServer(("127.0.0.1", 0), Handler)
