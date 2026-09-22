"""R3 source/build and validation fingerprints; no secrets or runtime data."""
import hashlib
import json
from r2_evidence import ROOT, source_digest


def inputs_digest():
    paths = [*sorted((ROOT/'tools/redesign').glob('r3_*.py')),
             ROOT/'tools/redesign/r2_probe.py', ROOT/'tools/redesign/r2_evidence.py',
             *sorted((ROOT/'testdata/redesign').glob('*.py')),
             ROOT/'tools/phase0/mock_openai.py',
             ROOT/'config/r3-routes.json', ROOT/'config/r3-capabilities.json',
             ROOT/'deploy/r3/compose.native.yml', ROOT/'deploy/r3/.env.example',
             ROOT/'scripts/r3-compose.sh', ROOT/'scripts/r3-install.sh', ROOT/'scripts/r2-install.sh',
             *sorted((ROOT/'tools/community-import').rglob('*.py')),
             *sorted(p for p in (ROOT/'tools/frontend-overlay').rglob('*') if p.is_file() and p.suffix in {'.mjs','.ts','.vue','.json','.css'})]
    digest = hashlib.sha256()
    for path in paths:
        digest.update(str(path.relative_to(ROOT)).encode()+b'\0'+path.read_bytes()+b'\0')
    return digest.hexdigest()


if __name__ == '__main__':
    target = ROOT / '.local/redesign-r3'
    report = {'gateway_sha256':hashlib.sha256((target/'gateway-linux').read_bytes()).hexdigest(),
              'gateway_source_sha256':source_digest()}
    (target/'gateway-build.json').write_text(json.dumps(report,indent=2)+'\n')
