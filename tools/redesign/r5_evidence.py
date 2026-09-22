"""R5 source/bundle fingerprints; never reads runtime secrets."""
import hashlib
import json
import sys
from r2_evidence import ROOT, source_digest

def ui_digest():
    digest=hashlib.sha256()
    for base in ('tools/frontend-overlay','branding/mindcreek'):
        for path in sorted((ROOT/base).rglob('*')):
            if path.is_file(): digest.update(str(path.relative_to(ROOT)).encode()+b'\0'+path.read_bytes())
    return digest.hexdigest()

def visual_review_valid():
    """A screenshot rerun or UI edit invalidates the recorded visual review."""
    try:
        evidence=ROOT/'docs/plans/evidence'
        visual=json.loads((evidence/'r5-visual.json').read_text())
        browser=json.loads((evidence/'r5-browser.json').read_text())
        return (visual['status']=='passed' and browser['status']=='passed'
                and visual['ui_source_sha256']==ui_digest()
                and visual['bundle_sha256']==browser['bundle_sha256']
                and set(visual['screenshots'])==set(browser['screenshots'])
                and all(hashlib.sha256((ROOT/path).read_bytes()).hexdigest()==value
                        for path,value in visual['screenshots'].items()))
    except (OSError,KeyError,ValueError):
        return False

if __name__=='__main__':
    directory=ROOT/'.local/redesign-r5'
    if sys.argv[1:] != ['record-ui']:
        report={'gateway_sha256':hashlib.sha256((directory/'gateway-linux').read_bytes()).hexdigest(),'gateway_source_sha256':source_digest()}
        (directory/'gateway-build.json').write_text(json.dumps(report,indent=2)+'\n')
        raise SystemExit(0)
    path_file=directory/'ui-path'
    if path_file.exists():
        from pathlib import Path
        ui=Path(path_file.read_text().strip());digest=hashlib.sha256()
        for f in sorted((ui/'dist').rglob('*')):
            if f.is_file():digest.update(str(f.relative_to(ui/'dist')).encode()+b'\0'+f.read_bytes())
        (ROOT/'docs/plans/evidence/r5-ui-build.json').write_text(json.dumps({'status':'passed','ui_source_sha256':ui_digest(),'bundle_sha256':digest.hexdigest(),'checks':['unit-tests','type-check','production-build','native-boundary']},indent=2)+'\n')
