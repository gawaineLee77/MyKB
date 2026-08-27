#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
UPSTREAM_FRONTEND="$REPO_ROOT/upstream/weknora/frontend"
TARGET=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-overlay.XXXXXX")
trap 'rm -rf "$TARGET"' EXIT HUP INT TERM

copy_anchor() {
  SOURCE="$1"
  mkdir -p "$TARGET/$(dirname -- "$SOURCE")"
  cp "$UPSTREAM_FRONTEND/$SOURCE" "$TARGET/$SOURCE"
}

copy_anchor index.html
copy_anchor embed.html
copy_anchor src/views/auth/Login.vue
copy_anchor src/components/menu.vue
copy_anchor src/router/index.ts
copy_anchor src/views/knowledge/KnowledgeBaseList.vue
copy_anchor src/utils/request.ts
copy_anchor src/assets/theme/theme.css
copy_anchor src/i18n/locales/en-US.ts
copy_anchor src/i18n/locales/zh-CN.ts

node "$SCRIPT_DIR/apply.mjs" "$TARGET" "$REPO_ROOT/branding/mindcreek"

grep -q '<title>MindCreek</title>' "$TARGET/index.html"
grep -q 'mindcreek-favicon.png' "$TARGET/index.html"
grep -q 'WeKnora_theme' "$TARGET/index.html"
grep -q 'mindcreek-mark.png' "$TARGET/src/views/auth/Login.vue"
grep -q 'mindcreek-wordmark' "$TARGET/src/components/menu.vue"
grep -q 'Welcome to MindCreek' "$TARGET/src/i18n/locales/en-US.ts"
grep -q '欢迎使用 MindCreek' "$TARGET/src/i18n/locales/zh-CN.ts"
grep -q 'WeKnora Cloud' "$TARGET/src/i18n/locales/en-US.ts"
grep -q 'MindCreek Stage 1 product theme' "$TARGET/src/assets/theme/theme.css"
grep -q 'mindcreekCreateKnowledgeSpace' "$TARGET/src/router/index.ts"
grep -q 'mindcreekNotesWorkspace' "$TARGET/src/router/index.ts"
grep -q 'mindcreekRAGWorkspace' "$TARGET/src/router/index.ts"
grep -q "router.push('/platform/mindcreek/create')" "$TARGET/src/views/knowledge/KnowledgeBaseList.vue"
grep -q "getProductProfile" "$TARGET/src/views/knowledge/KnowledgeBaseList.vue"
grep -q 'export function patch' "$TARGET/src/utils/request.ts"
grep -q 'Create a knowledge space' "$TARGET/src/mindcreek/CreateKnowledgeSpace.vue"
grep -q 'A quiet place for working notes' "$TARGET/src/mindcreek/NotesWorkspace.vue"
grep -q 'Managed Plain RAG preset' "$TARGET/src/mindcreek/RAGWorkspace.vue"
grep -q "index_profile: 'notes_plain'" "$TARGET/src/mindcreek/contracts.ts"
test -f "$TARGET/src/assets/img/mindcreek-mark.png"
test -f "$TARGET/public/mindcreek-favicon.png"
file "$TARGET/public/mindcreek-favicon.png" | grep -q 'PNG image data'

DIRTY=$(git -C "$REPO_ROOT/upstream/weknora" status --porcelain --untracked-files=all)
test -z "$DIRTY" || {
  echo "overlay check failed: upstream submodule is dirty" >&2
  exit 1
}

echo "MindCreek Stage 2 overlay verified; upstream remains unchanged"
