#!/usr/bin/env python3
"""Check the workspace design contract and local documentation links."""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
DESIGNS = [ROOT / "docs/OVERALL_DESIGN.md", ROOT / "docs/OVERALL_DESIGN_ZH.md"]
PLANS = ROOT / "docs/plans"
CURRENT = [*DESIGNS, ROOT / "README.md", ROOT / "docs/README.md",
           ROOT / "docs/CURRENT_STATUS.md", ROOT / "docs/ROADMAP.md",
           ROOT / "docs/archive/README.md",
           *[PLANS / name for name in ("WORKSPACE_CENTRIC_REDESIGN_ZH.md", "WORKSPACE_CENTRIC_R1.md", "WORKSPACE_CENTRIC_R1_MATRIX.md", "WORKSPACE_CENTRIC_R2.md", "WORKSPACE_CENTRIC_R2_ACCEPTANCE.md")]]
CURRENT.extend([ROOT / "deploy/r2/README.md", ROOT / "deploy/r3/README.md", *[PLANS / name for name in ("WORKSPACE_CENTRIC_R3_ACCEPTANCE.md", "WORKSPACE_CENTRIC_R3_ROUTES.md", "WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md", "WORKSPACE_CENTRIC_R4.md")]])
CURRENT.extend([ROOT / "deploy/r4/README.md", PLANS / "WORKSPACE_CENTRIC_R4_ACCEPTANCE.md", PLANS / "WORKSPACE_CENTRIC_R4_DEPENDENCIES.md"])
CURRENT.extend([ROOT / "docs/guides/R4_KNOWLEDGE_GRAPH_ZH.md", PLANS / "R4_KNOWLEDGE_GRAPH_ACCEPTANCE.md"])
CURRENT.extend([ROOT / "docs/guides/R4_ENTERPRISE_LINKS_ZH.md", PLANS / "R4_UI_LINKS_ACCEPTANCE.md"])
CURRENT.extend([ROOT / "docs/guides/R5_EMPLOYEE_ASSISTANT_ZH.md", PLANS / "WORKSPACE_CENTRIC_R5.md", PLANS / "WORKSPACE_CENTRIC_R5_ACCEPTANCE.md"])
HISTORY = [ROOT / "docs/archive/design" / name for name in ("overall-design-v0.7.md", "overall-design-v0.7-zh.md")]
GUIDES = [ROOT / "docs/guides" / name for name in ("FULL_DEPLOYMENT_RUNBOOK_ZH.md", "PHASE5_IDENTITY_PROVIDER.md", "PHASE5_OPERATIONS.md", "AGENT_AND_MCP.md", "COMMUNITY_IMPORT.md")]


def unfenced(text):
    return re.sub(r"^```[^\n]*\n.*?^```\s*$", "", text, flags=re.M | re.S)


def anchors(text):
    result = set(re.findall(r'<a\s+id="([^"]+)"', text))
    for heading in re.findall(r"^#{1,6}\s+(.+)$", unfenced(text), re.M):
        slug = re.sub(r"[^\w\s-]", "", heading.lower()).strip().replace(" ", "-")
        result.add(slug)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--r1", action="store_true", help="also validate recorded R1 evidence")
    parser.add_argument("--r2", action="store_true", help="also validate R2 implementation evidence")
    parser.add_argument("--r3", action="store_true", help="validate current R3 executable and interface evidence")
    parser.add_argument("--r4", action="store_true", help="verify current R4 UI/API and executable evidence")
    parser.add_argument("--r5", action="store_true", help="verify current employee-assistant evidence and preserved R4 history")
    args = parser.parse_args()
    errors = []
    texts = {}
    for path in [*CURRENT, *HISTORY, *GUIDES]:
        if not path.is_file():
            errors.append(f"missing {path.relative_to(ROOT)}")
            continue
        text = texts[path] = path.read_text()
        if len(re.findall(r"^```", text, re.M)) % 2:
            errors.append(f"unbalanced fences: {path.relative_to(ROOT)}")
        if path in CURRENT and any(line != line.rstrip() for line in text.splitlines()):
            errors.append(f"trailing whitespace: {path.relative_to(ROOT)}")
        for target in re.findall(r"\]\(([^)]+)\)", unfenced(text)):
            if re.match(r"[a-z]+:", target):
                continue
            file, _, fragment = unquote(target).partition("#")
            destination = (path.parent / file).resolve() if file else path
            if not destination.exists():
                errors.append(f"broken link in {path.relative_to(ROOT)}: {target}")
            elif fragment and destination.suffix == ".md" and fragment not in anchors(destination.read_text()):
                errors.append(f"missing anchor in {path.relative_to(ROOT)}: {target}")
    sequences = []
    required = ("0.8-workspace-r5", "v0.8.0", "Owner", "Admin", "Contributor", "Viewer", "OAuth2", "tenantless", "manage_members", "builtin-mindcreek-chat", "builtin-mindcreek-embedding", "builtin-mindcreek-rerank", "builtin-mindcreek-vlm", "MCP", "```mermaid")
    for path in DESIGNS:
        text = texts.get(path, "")
        for term in required:
            if term not in text:
                errors.append(f"required design contract absent: {path.name}: {term}")
        sequences.append(re.findall(r"^#{2,4} (\d+(?:\.\d+)*)\.? ", text, re.M))
        for stage in range(1, 7):
            if f"| R{stage} |" not in text:
                errors.append(f"missing R{stage} roadmap row in {path.name}")
    if not sequences[0] or sequences[0] != sequences[1]:
        errors.append("English/Chinese numbered sections differ")
    if "local admin" not in texts.get(DESIGNS[0], "").lower() or "本地 admin" not in texts.get(DESIGNS[1], ""):
        errors.append("independent local admin design absent")
    if args.r1:
        check_r1(errors)
    if args.r2:
        check_r2(errors)
    if args.r3:
        check_r3(errors)
    if args.r4:
        check_r4(errors)
    if args.r5:
        check_r5(errors)
    if errors:
        print("Design check failed:\n" + "\n".join(errors[:12]), file=sys.stderr)
        return 1
    print(f"Design documents verified: {len(sequences[0])} aligned sections; {len(texts)} documents; local links and workspace contracts")
    if args.r1:
        print("R1 evidence verified: API requests, expected checks, uncached gateway result and probe checksum")
    if args.r2:
        print("R2 historical evidence verified: immutable baseline hashes, API recovery and prior browser screenshots")
    if args.r3:
        print("R3 evidence verified: preserved API/MCP and executable evidence; current regression is recorded by R4")
    if args.r4:
        print("R4 evidence verified; after R5 begins, immutable historical hashes are checked")
    if args.r5:
        print("R5 evidence verified: current sources/build, API/browser/engineering checks and screenshots")
    return 0


def check_r1(errors):
    try:
        api = json.loads((PLANS / "evidence/r1-api-probe.json").read_text())
        gateway = json.loads((PLANS / "evidence/r1-gateway-tests.json").read_text())
        if api["status"] != "passed" or api.get("cleanup") != "only disposable project removed":
            errors.append("R1 API run or disposable cleanup incomplete")
        required = {"admin.register_tenantless", "admin.bootstrap_promotes_existing_user", "admin.creates_default_space_as_owner", "workspace.system_admin_has_no_native_creation_exemption", "workspace.native_self_service_allows_viewer_requires_gateway_policy", "employee.oauth_provisions_without_personal_space", "employee.duplicate_membership_conflicts_without_duplicate_row", "employee.removed_member_not_readded_by_native_login", "member.api_key_cannot_assign_owner", "member.contributor_creates_and_updates_own_agent", "member.viewer_creation_denied_native_creator_edit_preserved", "member.ownership_transfer", "admin.restart_does_not_reclaim_transferred_ownership", "embed.channel_token_survives_employee_removal_requires_adapter", "embed.channel_disable_revokes_token"}
        passed = {item["name"] for item in api["checks"] if item["passed"]}
        if required - passed or any(not item["passed"] for item in api["checks"]):
            errors.append("R1 required API evidence missing or failed")
        if not api.get("requests") or any(r["actual_status"] not in r["expected_status"] for r in api["requests"]):
            errors.append("R1 HTTP request evidence incomplete")
        if api["probe_sha256"] != hashlib.sha256((ROOT / "tools/redesign/r1_probe.py").read_bytes()).hexdigest():
            errors.append("R1 probe changed after evidence capture; rerun it")
        version = next(c for c in api["checks"] if c["name"] == "upstream.version")
        if version["version"] != "0.8.0" or version["commit_id"] != "1edcd54":
            errors.append("R1 live image does not match approved version/commit")
        if gateway["status"] != "passed" or gateway["cached"] or gateway["passed_packages"] < 1:
            errors.append("R1 uncached gateway evidence missing")
    except (OSError, KeyError, ValueError, StopIteration) as error:
        errors.append(f"R1 evidence cannot be verified: {error}")


def check_r2(errors):
    sys.path.insert(0, str(ROOT / "tools/redesign"))
    from r2_evidence import source_digest
    try:
        api = json.loads((PLANS / "evidence/r2-api-probe.json").read_text())
        ui = json.loads((PLANS / "evidence/r2-ui-probe.json").read_text())
        checks = json.loads((PLANS / "evidence/r2-checks.json").read_text())
        required = {"install.concurrent_default_space_single_id", "employee.concurrent_postgres_onboarding", "employee.removed_not_readded", "install.current_owner_repairs_revoked_key", "admin.logout_revokes_access_and_refresh", "member.last_owner_protected", "workspace.transferred_owner_has_no_platform_creation"}
        if api["status"] != "passed" or required - {c["name"] for c in api["checks"] if c["passed"]}:
            errors.append("R2 required API checks incomplete")
        if api["cleanup"] != "only disposable project removed" or any(r["actual_status"] not in r["expected_status"] for r in api["requests"]):
            errors.append("R2 request or cleanup evidence incomplete")
        # R2 reports are historical once R3 implementation starts; their hashes
        # are fixed in the R3 baseline. Current behavior is tested by R3 afresh.
        check_r2_history(errors)
        if ui["status"] != "passed" or len(ui["checks"]) < 9 or any(not c["passed"] for c in ui["checks"]):
            errors.append("R2 browser checks incomplete")
        if ui["probe_sha256"] != hashlib.sha256((ROOT / "tools/redesign/r2_ui_probe.mjs").read_bytes()).hexdigest():
            errors.append("R2 UI probe changed after verification")
        if len(ui["screenshots"]) != 5 or any(not (ROOT / p).is_file() for p in ui["screenshots"]):
            errors.append("R2 screenshot evidence incomplete")
        if checks["status"] != "passed" or checks["gateway_cached"] or not checks["screenshots_inspected"] or checks["node_major"] != 24:
            errors.append("R2 executable or visual verification incomplete")
    except (OSError, KeyError, ValueError) as error:
        errors.append(f"R2 evidence cannot be verified: {error}")


def check_r2_history(errors):
    baseline = json.loads((PLANS / "evidence/r3-baseline.json").read_text())
    for name, expected in baseline["r2_evidence_sha256"].items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            errors.append("R2 historical evidence changed: " + name)


def check_r3(errors):
    sys.path.insert(0, str(ROOT / "tools/redesign"))
    from r3_evidence import source_digest, inputs_digest
    if (PLANS / "evidence/r4-baseline.json").exists():
        check_r3_history(errors)
        return
    try:
        check_r2_history(errors)
        api = json.loads((PLANS / "evidence/r3-api-probe.json").read_text())
        checks = json.loads((PLANS / "evidence/r3-checks.json").read_text())
        for record in (api, checks):
            if record["status"] != "passed" or record["gateway_source_sha256"] != source_digest() or record["r3_inputs_sha256"] != inputs_digest():
                errors.append("R3 evidence failed or differs from current source/validation inputs")
        required = {"r3.kb.native_create_without_profile", "r3.document.manual_native_processing", "r3.document.native_multipart_upload", "r3.mcp.native_model_answer_and_bound_session", "r3.agent.native_share_and_revocation", "r3.share.revocation_denies_history_reconnect_and_model", "r3.identity.suspension_blocks_retrieval", "r3.migration.bindings_survive_gateway_restart", "r3.migration.populated_audit_refuses_rollback", "employee.removed_not_readded", "admin.logout_revokes_access_and_refresh"}
        if required - {c["name"] for c in api["checks"] if c["passed"]} or any(not c["passed"] for c in api["checks"]):
            errors.append("R3 required API cases missing")
        if api["cleanup"] != "only disposable project removed" or any(r["actual_status"] not in r["expected_status"] for r in api["requests"]):
            errors.append("R3 API statuses/cleanup incomplete")
        if not any("execution_counts" in r and any(k.startswith("native:") for k in r["execution_counts"]) for r in api["requests"]):
            errors.append("R3 native retrieval call evidence missing")
        expected = {"gateway-uncached", "gateway-race", "phase0", "phase5", "stage1", "frontend-node24", "importer-python312", "r3-route-manifest", "r3-compose", "r3-shell"}
        if expected != {c["name"] for c in checks["checks"] if c["exit_code"] == 0} or not checks["node"].startswith("v24.") or not checks["upstream_clean"]:
            errors.append("R3 engineering checks incomplete")
    except (OSError, KeyError, ValueError) as error:
        errors.append(f"R3 evidence cannot be verified: {error}")


def check_r3_history(errors):
    baseline = json.loads((PLANS / "evidence/r4-baseline.json").read_text())
    for name, expected in baseline["r3_evidence_sha256"].items():
        if hashlib.sha256((PLANS / "evidence" / name).read_bytes()).hexdigest() != expected:
            errors.append("R3 historical evidence changed: " + name)


def check_r4(errors):
    if (PLANS / "evidence/r5-baseline.json").exists():
        check_r4_history(errors)
        return
    sys.path.insert(0, str(ROOT / "tools/redesign"))
    from r4_evidence import source_digest, inputs_digest, ui_digest
    try:
        check_r2_history(errors); check_r3_history(errors)
        api = json.loads((PLANS / "evidence/r4-api-probe.json").read_text())
        checks = json.loads((PLANS / "evidence/r4-checks.json").read_text())
        browser = json.loads((PLANS / "evidence/r4-browser.json").read_text())
        build = json.loads((PLANS / "evidence/r4-ui-build.json").read_text())
        for record in (api, checks):
            if record["status"] != "passed" or record["gateway_source_sha256"] != source_digest() or record["r4_inputs_sha256"] != inputs_digest():
                errors.append("R4 evidence failed or source/validation inputs changed")
        if build["status"] != "passed" or build["ui_source_sha256"] != ui_digest(): errors.append("R4 UI build evidence changed")
        required = {"r4.browser.real_native_ui_integration", "r4.members.preview_owner_only", "r3.share.revocation_denies_history_reconnect_and_model", "r3.migration.bindings_survive_gateway_restart", "employee.removed_not_readded", "admin.logout_revokes_access_and_refresh"}
        if required - {c["name"] for c in api["checks"] if c["passed"]} or any(not c["passed"] for c in api["checks"]): errors.append("R4 API/R2/R3 regression missing")
        if api["cleanup"] != "only disposable project removed" or any(r["actual_status"] not in r["expected_status"] for r in api["requests"]): errors.append("R4 API status or cleanup incomplete")
        ui_cases = {"navigation.owner", "navigation.admin", "navigation.contributor", "navigation.viewer", "kb.native_create_and_settings", "kb.native_upload", "faq.native_page", "agent.native_editor", "viewer.native_chat_and_history", "members.batch_preview_and_repeat", "members.two_step_transfer_and_reload", "admin.real_login", "admin.create_switch_and_expiry", "roles.refresh_downgrade", "members.removed_relogin_restricted", "retired.bookmark"}
        if browser["status"] != "passed" or ui_cases - {c["name"] for c in browser["checks"] if c["passed"]} or any(not c["passed"] for c in browser["checks"]): errors.append("R4 actual browser workflows incomplete")
        if browser.get("ui_source_sha256") != ui_digest() or browser.get("bundle_sha256") != build["bundle_sha256"]: errors.append("R4 browser bundle/source differs from validated build")
        for role in ("owner", "admin", "contributor", "viewer"):
            for state in ("before", "after"):
                if not (ROOT / f"docs/assets/r4/{state}-{role}.png").is_file(): errors.append(f"R4 {state}/{role} screenshot missing")
        expected = {"gateway-uncached", "gateway-race", "phase0", "phase5", "stage1", "frontend-node24", "importer-python312", "r3-route-manifest", "r4-compose", "r4-shell"}
        if expected != {c["name"] for c in checks["checks"] if c["exit_code"] == 0} or not checks["node"].startswith("v24.") or not checks["upstream_clean"]: errors.append("R4 engineering checks incomplete")
    except (OSError, KeyError, ValueError) as error:
        errors.append(f"R4 evidence cannot be verified: {error}")


def check_r4_history(errors):
    check_r2_history(errors); check_r3_history(errors)
    baseline = json.loads((PLANS / "evidence/r5-baseline.json").read_text())
    for name, digest in baseline["r4_evidence_sha256"].items():
        if hashlib.sha256((PLANS / "evidence" / name).read_bytes()).hexdigest() != digest:
            errors.append("R4 historical evidence changed: " + name)


def check_r5(errors):
    sys.path.insert(0,str(ROOT / "tools/redesign"))
    from r5_evidence import source_digest, ui_digest, visual_review_valid
    try:
        check_r4_history(errors)
        records={name:json.loads((PLANS / f"evidence/r5-{name}.json").read_text()) for name in ("api-probe","browser","checks","ui-build")}
        for name, record in records.items():
            if record["status"] != "passed": errors.append("R5 failed evidence: " + name)
        for name in ("api-probe","checks"):
            if records[name]["gateway_source_sha256"] != source_digest(): errors.append("R5 gateway source changed: " + name)
        for name in ("browser","ui-build"):
            if records[name]["ui_source_sha256"] != ui_digest(): errors.append("R5 frontend source changed: " + name)
        if records['browser']['bundle_sha256'] != records['ui-build']['bundle_sha256']: errors.append('R5 browser bundle differs')
        api=records['api-probe']
        required={'r5.cross_origin_browser_and_synthetic_oauth','r5.denials_before_model_and_no_main_site_bypass','r5.concurrent_rate_budget','r5.restart_preserves_bindings_and_budget','r5.employee_logout_and_removal_revoked','r3.share.revocation_denies_history_reconnect_and_model','admin.logout_revokes_access_and_refresh'}
        if required - {c['name'] for c in api['checks'] if c['passed']}: errors.append('R5 API coverage incomplete')
        if api['cleanup']!='only disposable project removed' or any(r['actual_status'] not in r['expected_status'] for r in api['requests']): errors.append('R5 API request or cleanup failure')
        browser=records['browser']
        if len(browser['screenshots']) < 5 or any(not (ROOT/p).is_file() for p in browser['screenshots']): errors.append('R5 screenshot evidence incomplete')
        if browser['probe_sha256'] != hashlib.sha256((ROOT/'tools/redesign/r5_browser.mjs').read_bytes()).hexdigest(): errors.append('R5 browser probe changed')
        if not visual_review_valid(): errors.append('R5 screenshot review is missing or stale')
        if api['probe_sha256'] != hashlib.sha256((ROOT/'tools/redesign/r2_probe.py').read_bytes()).hexdigest(): errors.append('R5 API harness changed')
        checks=records['checks']
        if any(c['exit_code'] for c in checks['checks']) or not checks['node'].startswith('v24.') or not checks['upstream_clean'] or not checks['screenshots_inspected']: errors.append('R5 engineering/visual acceptance incomplete')
    except (OSError,KeyError,ValueError) as error:
        errors.append(f'R5 evidence cannot be verified: {error}')


if __name__ == "__main__":
    raise SystemExit(main())
