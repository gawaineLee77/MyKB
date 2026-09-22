"""Content fingerprints for R2 build and acceptance evidence; no configuration values."""
from pathlib import Path
import hashlib
import json
import sys

ROOT = Path(__file__).resolve().parents[2]

def source_digest():
    source = ROOT / 'services/gateway'
    digest = hashlib.sha256()
    for path in sorted(source.rglob('*')):
        if path.is_file() and (path.suffix in {'.go', '.sql'} or path.name in {'go.mod', 'go.sum'}):
            digest.update(str(path.relative_to(ROOT)).encode() + b'\0' + path.read_bytes() + b'\0')
    return digest.hexdigest()

if __name__ == '__main__':
    target = ROOT / '.local/redesign-r2'
    binary = target / 'gateway-linux'
    if sys.argv[1:] != ['record-build']:
        raise SystemExit('usage: r2_evidence.py record-build')
    report = {'gateway_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'gateway_source_sha256': source_digest()}
    (target / 'gateway-build.json').write_text(json.dumps(report, indent=2) + '\n')
