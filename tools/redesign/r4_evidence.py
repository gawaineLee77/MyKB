"""Current R4 source fingerprints; historical R2/R3 reports stay immutable."""
import hashlib
import json
from pathlib import Path
from r2_evidence import ROOT, source_digest
from r3_evidence import inputs_digest as r3_inputs_digest


def fingerprint(paths):
    digest = hashlib.sha256()
    for path in sorted(set(paths)):
        digest.update(str(path.relative_to(ROOT)).encode() + b'\0' + path.read_bytes() + b'\0')
    return digest.hexdigest()


def ui_digest():
    return fingerprint([p for p in (ROOT/'tools/frontend-overlay').rglob('*') if p.is_file() and p.suffix in {'.mjs','.ts','.vue','.json','.css','.sh'}] +
                       [ROOT/'images/mindcreek-ui/Dockerfile', ROOT/'scripts/r4-build-ui.sh'])


def inputs_digest():
    paths = [*sorted((ROOT/'tools/redesign').glob('r4_*')),
             *sorted((ROOT/'scripts').glob('r4-*.sh')),
             *sorted(p for p in (ROOT/'deploy/r4').iterdir() if p.is_file() and p.suffix != '.md'),
             ROOT/'scripts/check-design-docs.py']
    return hashlib.sha256((r3_inputs_digest() + ui_digest() + fingerprint([p for p in paths if p.is_file()])).encode()).hexdigest()


if __name__ == '__main__':
    target = ROOT/'.local/redesign-r4'
    (target/'gateway-build.json').write_text(json.dumps({
        'gateway_sha256':hashlib.sha256((target/'gateway-linux').read_bytes()).hexdigest(),
        'gateway_source_sha256':source_digest(),
    }, indent=2)+'\n')
