#!/usr/bin/env python3
"""Verify live Plain RAG creation and representative document ingestion."""

from __future__ import annotations

import argparse
import io
import json
import os
import runpy
import secrets
import sys
import time
import urllib.error
import urllib.request
import zipfile
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
SUPPORT = runpy.run_path(str(ROOT / "scripts/phase1-gate-c-probe.py"))
Client = SUPPORT["Client"]
APIError = SUPPORT["APIError"]
error_code = SUPPORT["error_code"]
login = SUPPORT["login"]
multipart_request = SUPPORT["multipart_request"]
wait_for_gateway = SUPPORT["wait_for_gateway"]


def office_archive(entries: dict[str, str]) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
        for name, content in entries.items():
            archive.writestr(name, content)
    return output.getvalue()


def docx_fixture(text: str) -> bytes:
    return office_archive(
        {
            "[Content_Types].xml": '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>',
            "_rels/.rels": '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>',
            "word/document.xml": f'<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>{text}</w:t></w:r></w:p><w:sectPr/></w:body></w:document>',
        }
    )


def xlsx_fixture(text: str) -> bytes:
    return office_archive(
        {
            "[Content_Types].xml": '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>',
            "_rels/.rels": '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
            "xl/workbook.xml": '<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Knowledge" sheetId="1" r:id="rId1"/></sheets></workbook>',
            "xl/_rels/workbook.xml.rels": '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>',
            "xl/worksheets/sheet1.xml": f'<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>{text}</t></is></c></row></sheetData></worksheet>',
        }
    )


def pdf_fixture(text: str) -> bytes:
    safe = text.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")
    stream = f"BT /F1 12 Tf 40 760 Td ({safe}) Tj ET".encode("ascii")
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
        b"<< /Length " + str(len(stream)).encode("ascii") + b" >>\nstream\n" + stream + b"\nendstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ]
    output = bytearray(b"%PDF-1.4\n")
    offsets = [0]
    for number, body in enumerate(objects, 1):
        offsets.append(len(output))
        output.extend(f"{number} 0 obj\n".encode("ascii") + body + b"\nendobj\n")
    xref = len(output)
    output.extend(f"xref\n0 {len(objects) + 1}\n0000000000 65535 f \n".encode("ascii"))
    for offset in offsets[1:]:
        output.extend(f"{offset:010d} 00000 n \n".encode("ascii"))
    output.extend(f"trailer\n<< /Size {len(objects) + 1} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode("ascii"))
    return bytes(output)


def wait_for_documents(client: Any, path: str, token: str, expected_ids: set[str], timeout: int = 180) -> dict[str, Any]:
    deadline = time.time() + timeout
    while time.time() < deadline:
        _, body = client.request("GET", path, token=token)
        documents = {str(item["id"]): item for item in body.get("data", {}).get("items", [])}
        if expected_ids.issubset(documents):
            terminal = {"completed", "failed", "cancelled"}
            if all(documents[item_id].get("parse_status") in terminal for item_id in expected_ids):
                return documents
        time.sleep(2)
    raise RuntimeError("representative Plain RAG fixtures did not reach terminal states")


def hybrid_search(client: Any, kb_id: str, token: str, query: str, sentinel: str) -> list[dict[str, Any]]:
    _, body = client.request(
        "POST",
        f"/api/v1/knowledge-bases/{kb_id}/hybrid-search",
        {"query_text": query, "vector_threshold": 0, "keyword_threshold": 0, "match_count": 10},
        token=token,
    )
    results = list(body.get("data", []))
    if sentinel not in json.dumps(results, ensure_ascii=False):
        raise RuntimeError(f"hybrid retrieval missed sentinel for query {query!r}")
    return results


def normal_chat(client: Any, token: str, session_id: str, kb_id: str, model_id: str, query: str) -> list[dict[str, Any]]:
    request = urllib.request.Request(
        client.base_url + f"/api/v1/knowledge-chat/{session_id}",
        data=json.dumps(
            {
                "query": query,
                "knowledge_base_ids": [kb_id],
                "summary_model_id": model_id,
                "disable_title": True,
                "channel": "web",
            }
        ).encode("utf-8"),
        headers={"Accept": "text/event-stream", "Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        method="POST",
    )
    events: list[dict[str, Any]] = []
    total_bytes = 0
    with urllib.request.urlopen(request, timeout=90) as response:
        if response.status != 200 or "text/event-stream" not in response.headers.get("Content-Type", ""):
            raise RuntimeError(f"normal chat did not open an SSE stream: {response.status}")
        for raw_line in response:
            total_bytes += len(raw_line)
            if total_bytes > 4 * 1024 * 1024:
                raise RuntimeError("normal chat returned an oversized stream")
            line = raw_line.decode("utf-8").strip()
            if not line.startswith("data:"):
                continue
            raw_data = line[5:].strip()
            if not raw_data or raw_data == "[DONE]":
                continue
            event = json.loads(raw_data)
            events.append(event)
            if event.get("response_type") == "error":
                raise RuntimeError(f"normal chat error: {event.get('content', 'unknown')}")
    if not any(event.get("response_type") == "answer" and event.get("content") for event in events):
        raise RuntimeError("normal chat returned no answer content")
    return events


def verify_openable_citations(client: Any, token: str, kb_id: str, events: list[dict[str, Any]], sentinel: str) -> int:
    references = [reference for event in events for reference in event.get("knowledge_references", [])]
    matching = [reference for reference in references if sentinel in str(reference.get("content", ""))]
    if not matching:
        raise RuntimeError("normal chat returned no sentinel citation")
    opened = 0
    for reference in matching[:2]:
        knowledge_id = str(reference.get("knowledge_id", ""))
        chunk_id = str(reference.get("id", ""))
        if not knowledge_id or not chunk_id or str(reference.get("knowledge_base_id", kb_id)) != kb_id:
            raise RuntimeError("normal chat returned an incomplete citation")
        _, knowledge_body = client.request("GET", f"/api/v1/knowledge/{knowledge_id}", token=token)
        _, chunk_body = client.request("GET", f"/api/v1/chunks/by-id/{chunk_id}", token=token)
        if str(knowledge_body.get("data", {}).get("id")) != knowledge_id or str(chunk_body.get("data", {}).get("id")) != chunk_id:
            raise RuntimeError("citation target could not be opened")
        opened += 1
    return opened


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--report", default=str(ROOT / ".local/phase1-gate-d-report.json"))
    arguments = parser.parse_args()
    client = Client(arguments.base_url)
    wait_for_gateway(client, arguments.health_timeout)

    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    email = f"gate-d-owner-{nonce}@example.invalid"
    password = f"GateD-{secrets.token_urlsafe(12)}!"
    client.request("POST", "/api/v1/auth/register", {"username": f"gate-d-{nonce}", "email": email, "password": password}, allowed=(201,))
    token = str(login(client, email, password)["token"])
    _, model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"gate-d-embedding-{nonce}",
            "type": "Embedding",
            "source": "remote",
            "description": "Deterministic Gate D embedding model",
            "parameters": {"base_url": "http://mock-embedding:19090/v1", "api_key": "gate-d-not-a-secret", "provider": "generic", "embedding_parameters": {"dimension": 64, "truncate_prompt_tokens": 0}},
        },
        token=token,
        allowed=(201,),
    )
    _, chat_model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"gate-d-chat-{nonce}",
            "type": "KnowledgeQA",
            "source": "remote",
            "description": "Deterministic Gate D chat model",
            "parameters": {"base_url": "http://mock-embedding:19090/v1", "api_key": "gate-d-not-a-secret", "provider": "generic", "temperature": 0},
            "is_default": True,
        },
        token=token,
        allowed=(201,),
    )
    chat_model_id = str(chat_model_body["data"]["id"])
    _, space_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        {"mode": "rag", "index_profile": "plain", "name": f"Gate D Plain RAG {nonce}", "description": "Synthetic multi-format ingestion fixtures", "embedding_model_id": str(model_body["data"]["id"]), "summary_model_id": chat_model_id, "storage_provider": "local"},
        token=token,
        allowed=(201,),
        headers={"Idempotency-Key": f"gate-d-rag-{nonce}"},
    )
    kb_id = str(space_body["data"]["knowledge_base_id"])
    base = f"/api/v1/knowledge-bases/{kb_id}/ingestions"
    sentinel = f"MINDCREEK_GATE_D_{nonce}"
    fixtures = {
        "gate-d.md": f"# MindCreek\n\nEnglish {sentinel} and 中文知识哨兵。".encode(),
        "gate-d.pdf": pdf_fixture(f"PDF {sentinel}"),
        "gate-d.docx": docx_fixture(f"Word {sentinel}"),
        "gate-d.xlsx": xlsx_fixture(f"Spreadsheet {sentinel}"),
    }
    document_ids: set[str] = set()
    for filename, content in fixtures.items():
        status, body = multipart_request(client, base, filename, content, token, (202,))
        if status != 202:
            raise RuntimeError(f"{filename} was not accepted")
        document_ids.add(str(body["data"]["id"]))

    documents = wait_for_documents(client, base, token, document_ids)
    failures = {item.get("file_name", item_id): item.get("error_message", "failed") for item_id, item in documents.items() if item_id in document_ids and item.get("parse_status") != "completed"}
    if failures:
        raise RuntimeError(f"representative fixture failures: {failures}")

    xlsx_id = next(item_id for item_id, item in documents.items() if item.get("file_name") == "gate-d.xlsx")
    client.request("POST", f"{base}/{xlsx_id}/retry", token=token, allowed=(202,))
    cancel_status, cancel_body = client.request("POST", f"{base}/{xlsx_id}/cancel", token=token, allowed=(202, 409))
    cancel_result = "accepted" if cancel_status == 202 else f"race:{error_code(cancel_body)}"
    if cancel_status == 202:
        client.request("POST", f"{base}/{xlsx_id}/retry", token=token, allowed=(202,))
        documents = wait_for_documents(client, base, token, {xlsx_id})
        if documents[xlsx_id].get("parse_status") != "completed":
            raise RuntimeError("cancelled spreadsheet did not complete after retry")

    english_results = hybrid_search(client, kb_id, token, sentinel, sentinel)
    chinese_results = hybrid_search(client, kb_id, token, "中文知识哨兵", "中文知识哨兵")
    citation_count = 0
    for language, query, expected in (("English", sentinel, sentinel), ("Chinese", "中文知识哨兵", "中文知识哨兵")):
        _, session_body = client.request(
            "POST",
            "/api/v1/sessions",
            {"title": f"Gate D {language}", "description": "Plain RAG normal-chat citation probe"},
            token=token,
            allowed=(201,),
        )
        events = normal_chat(client, token, str(session_body["data"]["id"]), kb_id, chat_model_id, query)
        citation_count += verify_openable_citations(client, token, kb_id, events, expected)

    report = {
        "plain_rag_profile": "pass",
        "markdown_pdf_word_spreadsheet": "pass",
        "processing_state_and_failure_field": "pass",
        "retry": "pass",
        "cancel": cancel_result,
        "hybrid_retrieval": {"english_hits": len(english_results), "chinese_hits": len(chinese_results)},
        "normal_chat": "pass",
        "openable_citations": citation_count,
        "graph_pixel_neo4j_required": False,
        "kb_id": kb_id,
        "sentinel": sentinel,
    }
    report_path = Path(arguments.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Gate D passed: multi-format Plain RAG, hybrid retrieval, normal chat, and openable citations")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Gate D probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
