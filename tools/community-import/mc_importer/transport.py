"""Bounded HTTPS requests with explicit credentials and no implicit redirects."""

from __future__ import annotations

import json
import http.client
import os
import socket
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

from .common import ImportFailure, select, substitute


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def transport_error(exc: Exception) -> str:
    reason = exc.reason if isinstance(exc, urllib.error.URLError) else exc
    if isinstance(reason, ssl.SSLCertVerificationError):
        return "http.tls_verification_failed"
    if isinstance(reason, ssl.SSLError):
        return "http.tls_failed"
    if isinstance(reason, (TimeoutError, socket.timeout)):
        return "http.timeout"
    if isinstance(reason, socket.gaierror):
        return "http.dns_failed"
    if isinstance(reason, ConnectionRefusedError):
        return "http.connection_refused"
    return "http.transport_failed"


def origin(url: str) -> str:
    try:
        parsed = urllib.parse.urlsplit(url)
        port = parsed.port or (443 if parsed.scheme == "https" else 80)
        host = parsed.hostname
    except ValueError as exc:
        raise ImportFailure("http.url_invalid") from exc
    if not host or parsed.username or parsed.password or parsed.fragment:
        raise ImportFailure("http.url_invalid")
    return f"{parsed.scheme}://{host.lower()}:{port}"


class Transport:
    def __init__(self, config: dict[str, Any]):
        settings = config.get("http", {})
        self.timeout = float(settings.get("timeout_seconds", 60))
        self.attempts = int(settings.get("read_attempts", 3))
        self.allow_http = settings.get("allow_insecure_http", False) is True
        if not 0 < self.timeout <= 1800 or not 1 <= self.attempts <= 5:
            raise ImportFailure("config.http_limits_invalid")
        context = ssl.create_default_context()
        ca_file = settings.get("ca_file")
        if ca_file:
            context.load_verify_locations(cafile=ca_file)
        # urllib honors HTTPS_PROXY/HTTP_PROXY/NO_PROXY; TLS verification stays on.
        self.opener = urllib.request.build_opener(NoRedirect(), urllib.request.HTTPSHandler(context=context))

    def validate_url(self, url: str) -> None:
        origin(url)
        parsed = urllib.parse.urlsplit(url)
        if "REPLACE_" in url or parsed.hostname.endswith(".invalid"):
            raise ImportFailure("config.endpoint_placeholder")
        if parsed.scheme not in (("http", "https") if self.allow_http else ("https",)):
            raise ImportFailure("http.https_required")

    def headers(self, request: dict[str, Any]) -> dict[str, str]:
        result = {"Accept": "application/json"}
        for name, value in request.get("headers", {}).items():
            if name.lower() in {"authorization", "cookie", "proxy-authorization", "x-api-key"}:
                raise ImportFailure("config.credentials_must_use_environment")
            result[name] = str(value)
        for name, variable in request.get("headers_env", {}).items():
            value = os.environ.get(variable, "")
            if not value:
                raise ImportFailure("config.credential_environment_missing")
            result[name] = value
        for name, value in result.items():
            if "\r" in name + value or "\n" in name + value:
                raise ImportFailure("config.header_invalid")
        return result

    def request_spec(self, spec: dict[str, Any], variables: dict[str, Any]) -> urllib.request.Request:
        escaped = {key: urllib.parse.quote(str(value), safe="") for key, value in variables.items()}
        url = substitute(spec.get("url", ""), escaped)
        self.validate_url(url)
        method = spec.get("method", "GET").upper()
        if method not in ("GET", "POST"):
            raise ImportFailure("config.read_method_invalid")
        query = substitute(spec.get("query", {}), variables)
        if query:
            url += ("&" if "?" in url else "?") + urllib.parse.urlencode(query, doseq=True)
        headers, body = self.headers(spec), None
        if "json" in spec and "form" in spec:
            raise ImportFailure("config.request_body_ambiguous")
        if "json" in spec:
            body = json.dumps(substitute(spec["json"], variables)).encode()
            headers["Content-Type"] = "application/json"
        elif "form" in spec:
            body = urllib.parse.urlencode(substitute(spec["form"], variables), doseq=True).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        if body is not None and method != "POST":
            raise ImportFailure("config.get_body_denied")
        return urllib.request.Request(url, data=body, headers=headers, method=method)

    def transfer(self, request: urllib.request.Request, limit: int, *, destination: Path | None = None,
                 retry_read: bool = False) -> tuple[bytes, dict[str, str]]:
        self.validate_url(request.full_url)
        attempts = self.attempts if retry_read else 1
        for attempt in range(attempts):
            try:
                with self.opener.open(request, timeout=self.timeout) as response:
                    headers = {name.lower(): value for name, value in response.headers.items()}
                    total, chunks = 0, []
                    if destination is not None:
                        handle = destination.open("wb")
                        os.chmod(destination, 0o600)
                    else:
                        handle = None
                    try:
                        while block := response.read(min(1024 * 1024, limit - total + 1)):
                            total += len(block)
                            if total > limit:
                                raise ImportFailure("http.response_too_large")
                            if handle:
                                handle.write(block)
                            else:
                                chunks.append(block)
                    finally:
                        if handle:
                            handle.close()
                    if total == 0:
                        raise ImportFailure("http.empty_response")
                    if headers.get("content-length", "").isdigit() and total != int(headers["content-length"]):
                        raise ImportFailure("http.truncated_response")
                    return b"".join(chunks), headers
            except urllib.error.HTTPError as exc:
                code = exc.code
                exc.close()
                if code not in (408, 429, 500, 502, 503, 504) or attempt == attempts - 1:
                    raise ImportFailure("http.status_error", code) from exc
            except (urllib.error.URLError, TimeoutError, socket.timeout, ConnectionError, OSError, http.client.HTTPException) as exc:
                if attempt == attempts - 1:
                    raise ImportFailure(transport_error(exc)) from exc
            time.sleep(min(2 ** attempt, 8))
        raise ImportFailure("http.attempts_exhausted")

    def json_request(self, spec: dict[str, Any], variables: dict[str, Any], limit: int = 16 * 1024 * 1024) -> Any:
        raw, _ = self.transfer(self.request_spec(spec, variables), limit, retry_read=True)
        try:
            return json.loads(raw)
        except (ValueError, UnicodeError) as exc:
            raise ImportFailure("http.invalid_json") from exc


class S3Downloader:
    def __init__(self, transport: Transport, config: dict[str, Any]):
        self.transport, self.config = transport, config.get("s3", {})

    def download(self, uuid: str, destination: Path, limit: int) -> dict[str, str]:
        # Article content can only select an opaque ID, never a request URL/host.
        if not isinstance(uuid, str) or not uuid or len(uuid) > 512 or any(c in uuid for c in "/\\:?&#\r\n"):
            raise ImportFailure("s3.uuid_invalid")
        spec = self.config.get("request", {})
        variables = {"s3uuid": uuid}
        mode = self.config.get("response_mode", "binary")
        if mode == "binary":
            request = self.transport.request_spec(spec, variables)
        elif mode == "json_url":
            response = self.transport.json_request(spec, variables)
            url = select(response, self.config.get("url_path", "data.url"))
            if not isinstance(url, str):
                raise ImportFailure("s3.download_url_invalid")
            allowed = {origin(item) for item in self.config.get("download_origins", [])}
            if origin(url) not in allowed:
                raise ImportFailure("s3.download_origin_denied")
            # Never forward API authorization to a signed file URL.
            request = urllib.request.Request(url, headers={"Accept": "application/octet-stream"})
        else:
            raise ImportFailure("config.s3_response_mode_invalid")
        _, headers = self.transport.transfer(request, limit, destination=destination, retry_read=True)
        return headers
