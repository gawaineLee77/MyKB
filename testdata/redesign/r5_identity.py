"""Synthetic browser IdP; only for the disposable R5 loopback deployment."""
import html
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlsplit, urlencode

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_): pass
    def send(self, data, kind='application/json'):
        data = data.encode()
        self.send_response(200); self.send_header('Content-Type',kind); self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
    def do_POST(self):
        form=parse_qs(self.rfile.read(int(self.headers.get('Content-Length','0'))).decode())
        self.send(json.dumps({'access_token':form.get('code',['employee'])[0],'token_type':'Bearer'}))
    def do_GET(self):
        url=urlsplit(self.path);q=parse_qs(url.query)
        if url.path=='/authorize':
            target=q.get('redirect_uri',[''])[0]
            if target not in ('http://mindcreek.localhost:18685','http://mindcreek.localhost:18685/api/v1/mindcreek/oidc/callback'): self.send_error(400);return
            if 'employee' in q:
                self.send_response(302);self.send_header('Location',target+'?'+urlencode({'code':q['employee'][0],'state':q['state'][0]}));self.end_headers();return
            hidden=''.join(f'<input type="hidden" name="{html.escape(k)}" value="{html.escape(v[0])}">' for k,v in q.items())
            self.send('<!doctype html><meta charset="utf-8"><h1>合成企业身份服务</h1><form>'+hidden+'<label>员工标识<input name="employee" value="r5-browser-alice" required></label><button>登录</button></form>','text/html; charset=utf-8');return
        subject=self.headers.get('Authorization','Bearer employee').removeprefix('Bearer ')
        self.send(json.dumps({'sub':subject,'name':subject,'email':subject+'@example.invalid'}))

HTTPServer(('0.0.0.0',18000),Handler).serve_forever()
