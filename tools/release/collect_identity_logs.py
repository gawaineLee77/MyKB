#!/usr/bin/env python3
"""Print only the gateway's structured authentication diagnostic events."""
import argparse
import json
from pathlib import Path
import subprocess
import sys

# Keep this list explicit. Never pass through arbitrary response/header fields.
FIELDS = set('event time request_id instance_id flow_id call_id stage reason status duration_ms method path route_class context_state error_kind error_code upstream_status authorization_method state_required pkce_enabled cookie_secure cookie_path ttl_seconds state_present cookie_present code_present transaction_found transaction_age_ms grant_found next_path endpoint_origin protocol token_request_format client_auth_method userinfo_token_transport subject_tenant_scoped subject_claim tenant_claim username_claim email_claim authorization_origin token_origin userinfo_origin redirect_origin redirect_target transaction_storage transaction_ttl_seconds ca_bundle_configured ca_bundle_readable ca_bundle_sha256 ca_certificate_count access_token_present subject_present tenant_present'.split())


def extract(line):
    start = line.find('{')
    if start < 0:
        return None
    try:
        event, _ = json.JSONDecoder().raw_decode(line[start:])
    except ValueError:
        return None
    if not isinstance(event, dict):
        return None
    name = event.get('event', '')
    if not isinstance(name, str) or not (name.startswith('identity_') or name == 'http_request' and (
        str(event.get('path', '')).startswith('/api/v1/auth/') or
        str(event.get('path', '')).startswith('/api/v1/mindcreek/oidc/')
    )):
        return None
    return {key: value for key, value in event.items() if key in FIELDS}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=Path, required=True)
    parser.add_argument('--since', default='15m')
    args = parser.parse_args()
    result = subprocess.run([sys.executable, str(args.package.resolve() / 'bin/mindcreek'), 'compose',
                             'logs', '--no-color', '--since', args.since, 'gateway'], text=True, capture_output=True)
    if result.returncode:
        raise SystemExit('Could not read gateway logs; check instance environment and Docker access.')
    count = 0
    for line in result.stdout.splitlines():
        event = extract(line)
        if event is not None:
            print(json.dumps(event, ensure_ascii=False))
            count += 1
    if not count:
        print('No diagnostic events found. Check the gateway image, project and --since window.', file=sys.stderr)


if __name__ == '__main__':
    main()
