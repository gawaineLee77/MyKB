#!/usr/bin/env python3
"""Build/check the R3 exact method/path manifest from the pinned router overlay."""
import argparse
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
TARGET = ROOT / "config/r3-routes.json"


def manifest(inventory):
    old = json.loads((ROOT / "config/phase1-route-policy.json").read_text())
    result = []
    for item in inventory:
        method, path = item["method"], item["path"]
        rule = next(r for r in old["rules"] if re.search(r["path_regex"], path))
        disposition = "disabled" if rule["classification"] == "disabled" else "native"
        # The optional graph extension applies dependency/model checks before
        # forwarding these exact native preview routes with the caller's identity.
        if method == "POST" and path in ["/api/v1/initialization/extract/" + action
                                        for action in ("fabri-tag", "fabri-text", "text-relation")]:
            disposition = "native"
        checks = []
        if disposition == "native":
            if path.startswith(("/api/v1/sessions", "/api/v1/messages", "/api/v1/knowledge-chat", "/api/v1/agent-chat", "/api/v1/knowledge-search")):
                checks.append("scope_session")
            if path.endswith("/hybrid-search"):
                checks.append("scope_session")
            if path.startswith("/api/v1/tenants/") and method == "PUT":
                checks.append("feature_settings")
            if path.startswith(("/api/v1/knowledge-bases", "/api/v1/agents", "/api/v1/knowledge-chat", "/api/v1/agent-chat", "/api/v1/sessions", "/api/v1/models", "/api/v1/initialization")):
                checks.append("models_upload")
            if path.startswith(("/api/v1/auth", "/api/v1/tenants", "/api/v1/me/invitations", "/api/v1/system/admin")):
                checks.append("enterprise")
            if path.startswith("/api/v1/agents") and method in ("POST", "PUT", "PATCH"):
                checks.append("agent_binding")
        result.append({"method": method, "path": path, "disposition": disposition, "checks": checks, "baseline_rule": rule["id"], "handler": item["handler"]})
    return {"schema_version": 1, "upstream_tag": old["upstream_tag"], "upstream_commit": old["upstream_commit"], "routes": result}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inventory", type=Path, required=True)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    value = manifest(json.loads(args.inventory.read_text()))
    assert len(value["routes"]) == 429
    text = json.dumps(value, indent=2, ensure_ascii=False) + "\n"
    if args.check:
        assert TARGET.read_text() == text, "R3 route manifest differs from pinned runtime inventory"
    else:
        TARGET.write_text(text)
    print(f"R3 route manifest: {len(value['routes'])} exact method/path entries")


if __name__ == "__main__":
    main()
