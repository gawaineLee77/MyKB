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
copy_anchor nginx.conf
copy_anchor src/views/auth/Login.vue
copy_anchor src/components/menu.vue
copy_anchor src/components/UserMenu.vue
copy_anchor src/components/Input-field.vue
copy_anchor src/router/index.ts
copy_anchor src/utils/request.ts
copy_anchor src/views/settings/ModelSettings.vue
copy_anchor src/assets/theme/theme.css
copy_anchor src/i18n/locales/en-US.ts
copy_anchor src/i18n/locales/zh-CN.ts

node "$SCRIPT_DIR/apply.mjs" "$TARGET" "$REPO_ROOT/branding/mindcreek"

grep -q '<title>MindCreek</title>' "$TARGET/index.html"
grep -q 'mindcreek-favicon.png' "$TARGET/index.html"
grep -q 'mindcreekOAuthRootCallback' "$TARGET/index.html"
grep -q 'location = /mcp' "$TARGET/nginx.conf"
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
grep -q 'mindcreekAskWorkspace' "$TARGET/src/router/index.ts"
grep -q 'mindcreekAdvancedModelSettings' "$TARGET/src/router/index.ts"
grep -q 'ManagedModelSettings' "$TARGET/src/views/settings/ModelSettings.vue"
grep -q 'chooseChatModel' "$TARGET/src/components/Input-field.vue"
grep -q 'mindcreek/AuthEntry.vue' "$TARGET/src/router/index.ts"
grep -q 'mindcreek/KnowledgeLibrary.vue' "$TARGET/src/router/index.ts"
grep -q 'export function patch' "$TARGET/src/utils/request.ts"
grep -q 'Create a knowledge space' "$TARGET/src/mindcreek/CreateKnowledgeSpace.vue"
grep -q 'Managed AI is ready' "$TARGET/src/mindcreek/CreateKnowledgeSpace.vue"
grep -q 'Workspace model overrides' "$TARGET/src/mindcreek/AdvancedModelSettings.vue"
grep -q 'Sign in with your organization' "$TARGET/src/mindcreek/SSOLogin.vue"
grep -q 'publicBrokerAuthorizationURL' "$TARGET/src/mindcreek/SSOLogin.vue"
grep -q 'configuration/network failure must not expose' "$TARGET/src/mindcreek/AuthEntry.vue"
grep -q '/api/v1/mindcreek/oidc/logout' "$TARGET/src/components/UserMenu.vue"
grep -q '/api/v1/mindcreek/models' "$TARGET/src/mindcreek/api.ts"
grep -q '/api/v1/mindcreek/models/.*test' "$TARGET/src/mindcreek/api.ts"
grep -q 'Organization-managed defaults' "$TARGET/src/mindcreek/ManagedModelSettings.vue"
! grep -q "@/api/model" "$TARGET/src/mindcreek/api.ts"
grep -q 'A quiet place for working notes' "$TARGET/src/mindcreek/NotesWorkspace.vue"
grep -q 'Managed Plain RAG preset' "$TARGET/src/mindcreek/RAGWorkspace.vue"
grep -q 'Shared with me' "$TARGET/src/mindcreek/KnowledgeLibrary.vue"
grep -q 'Share knowledge base' "$TARGET/src/mindcreek/SharingDialog.vue"
grep -q 'Publish knowledge base' "$TARGET/src/mindcreek/PublicationDialog.vue"
grep -q 'Organization public' "$TARGET/src/mindcreek/CatalogView.vue"
grep -q 'data-testid="subscribed-tab"' "$TARGET/src/mindcreek/KnowledgeLibrary.vue"
grep -q 'data-testid="discover-tab"' "$TARGET/src/mindcreek/KnowledgeLibrary.vue"
grep -q 'data-testid="ask-button"' "$TARGET/src/mindcreek/KnowledgeLibrary.vue"
grep -q 'Ask across the knowledge you choose' "$TARGET/src/mindcreek/AskWorkspace.vue"
grep -q 'resolveAgentScope' "$TARGET/src/mindcreek/AskWorkspace.vue"
grep -q "index_profile: 'notes_plain'" "$TARGET/src/mindcreek/contracts.ts"
test -f "$TARGET/src/assets/img/mindcreek-mark.png"
test -f "$TARGET/public/mindcreek-favicon.png"
file "$TARGET/public/mindcreek-favicon.png" | grep -q 'PNG image data'

DIRTY=$(git -C "$REPO_ROOT/upstream/weknora" status --porcelain --untracked-files=all)
test -z "$DIRTY" || {
  echo "overlay check failed: upstream submodule is dirty" >&2
  exit 1
}

echo "MindCreek Phase 5 overlay verified; upstream remains unchanged"
