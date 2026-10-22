// Exact adapters apply only to the disposable frontend copy, never the submodule.
export function applyKnowledgeBaseACL({ read, write, replaceExact: exact, replaceRegex: regex }) {
  const kbAPI = 'src/api/knowledge-base/index.ts'
  write(kbAPI, "import { createPrivateKnowledgeBase } from '@/mindcreek/kb-creation';\n" + read(kbAPI))
  exact(kbAPI, 'return post(`/api/v1/knowledge-bases`, data);', 'return createPrivateKnowledgeBase(data);')
  const request = 'src/utils/request.ts'
  exact(request, '    return Promise.reject({ \n      status,', `    if ([403, 404].includes(status) && typeof data?.error?.code === 'string' && data.error.code.startsWith('kb_acl.') && !originalRequest?.url?.includes('/kb-permissions/')) window.dispatchEvent(new Event('mindcreek-kb-permissions-changed'));\n    return Promise.reject({ \n      status,`)
  const kb = 'src/views/knowledge/KnowledgeBase.vue'
  const context = path => exact(path, '<script setup lang="ts">', `<script setup lang="ts">\nimport { useKBPermissionsContext } from '@/mindcreek/kb-permissions';\nconst kbAcl = useKBPermissionsContext();`)
  exact(kb, '<script setup lang="ts">', `<script setup lang="ts">\nimport { useKBPermissions, provideKBPermissions } from '@/mindcreek/kb-permissions';\nimport KBAccessPanel from '@/mindcreek/KBAccessPanel.vue';\nimport KBEffectiveNotice from '@/mindcreek/KBEffectiveNotice.vue';`)
  exact(kb, 'const kbInfo = ref<any>(null);', `const kbInfo = ref<any>(null);\nconst kbAcl = useKBPermissions(() => kbId.value);\nprovideKBPermissions(kbAcl);\nconst kbAccessOpen = ref(false);`)
  exact(kb, '  <template v-if="!isFAQ">', '  <KBEffectiveNotice /><t-alert v-if="kbAcl.error.value" theme="warning" :message="kbAcl.error.value" /><t-loading v-if="kbAcl.loading.value" text="正在核验知识库权限" />\n  <template v-if="kbAcl.visible.value && !isFAQ">')
  exact(kb, '  <template v-else>\n    <div class="faq-manager-wrapper">', '  <template v-else-if="kbAcl.visible.value">\n    <div class="faq-manager-wrapper">')
  for (const [name, value] of [['canEdit', "kbAcl.can('document.update')"], ['canManage', 'kbAcl.canManage()'], ['canMutateKnowledge', "kbAcl.can('document.delete') || kbAcl.can('document.update')"], ['canDownloadKnowledge', "kbAcl.can('document.download')"]]) {
    regex(kb, new RegExp(`const ${name} = computed\\(\\(\\) => \\{[\\s\\S]*?\\n\\}\\);`, 'g'), `const ${name} = computed(() => ${value});`)
  }
  exact(kb, '<t-tooltip v-if="canManage" :content="$t(\'knowledgeBase.settings\')"', '<t-button v-if="canManage" variant="text" @click="kbAccessOpen = true">访问与协作</t-button>\n              <t-tooltip v-if="canManage" :content="$t(\'knowledgeBase.settings\')"')
  exact(kb, '<div v-if="canEdit" class="doc-filter-actions">', '<div v-if="kbAcl.can(\'document.create\')" class="doc-filter-actions">')
  exact(kb, ':loading="folderTreeLoading" :can-edit="canEdit"', ':loading="folderTreeLoading" :can-edit="canManage"')
  exact(kb, '<div v-if="canEdit" class="tag-filter-panel__footer">', '<div v-if="canManage" class="tag-filter-panel__footer">')
  exact(kb, ':can-manage="canEdit"', ':can-manage="canManage"', 2)
  exact(kb, 'enabled: computed(() => canEdit.value && !isFAQ.value && cardList.value.length > 0)', 'enabled: computed(() => canMutateKnowledge.value && !isFAQ.value && cardList.value.length > 0)')
  exact(kb, '    && canEdit.value\n    && !docListLoading.value', "    && kbAcl.can('document.create')\n    && !docListLoading.value")
  exact(kb, '  <!-- 知识库编辑器（创建/编辑统一组件） -->', `  <t-dialog v-model:visible="kbAccessOpen" header="知识库访问与协作" width="min(1080px, 94vw)" top="5vh" :footer="false" destroy-on-close><KBAccessPanel v-if="kbAccessOpen && canManage" :kb-id="kbId" /></t-dialog>\n  <!-- 知识库编辑器（创建/编辑统一组件） -->`)
  exact(kb, 'v-if="missingStorageEngine" class="storage-engine-warning"', 'v-if="missingStorageEngine && canManage" class="storage-engine-warning"')
  exact(kb, 'v-if="unsupportedFileTypes.length" class="parser-hint"', 'v-if="unsupportedFileTypes.length && canManage" class="parser-hint"')
  exact(kb, '<p class="document-subtitle">{{ $t(\'knowledgeEditor.document.subtitle\') }}</p>', '<p class="document-subtitle">{{ kbAcl.can(\'document.create\') ? $t(\'knowledgeEditor.document.subtitle\') : \'阅读当前获准的解析内容；操作权限由知识库管理者授予。\' }}</p>')
  exact(kb, 'const ensureDocumentKbReady = () => {', "const ensureDocumentKbReady = () => {\n  if (!kbAcl.can('document.create')) { MessagePlugin.warning('当前未获新增文档权限。'); return false; }")
  exact(kb, 'const handleOpenKBSettings = () => {', 'const handleOpenKBSettings = () => {\n  if (!kbAcl.canManage()) return;')

  const editor = 'src/views/knowledge/KnowledgeBaseEditorModal.vue'
  exact(editor, '<script setup lang="ts">', `<script setup lang="ts">\nimport KBAccessPanel from '@/mindcreek/KBAccessPanel.vue';\nimport { useKBPermissions } from '@/mindcreek/kb-permissions';`)
  exact(editor, '<KBShareSettings :kb-id="activeKbId" :can-share="canShareKB" />', '<KBAccessPanel v-if="canShareKB" :kb-id="activeKbId" />')
  regex(editor, /const canShareKB = computed\(\(\) => \{[\s\S]*?\n\}\)/g, 'const editorAcl = useKBPermissions(() => activeKbId.value || undefined)\nconst canShareKB = computed(() => editorAcl.canManage())')
  exact(editor, '<div v-if="formData" class="section-content">', '<div v-if="formData" class="section-content">\n                    <t-alert v-if="editorMode === \'create\'" theme="info" message="新建默认仅创建者和空间 Owner/Admin 可见。创建后可在「访问与协作」中指定成员或向空间公开。" />')
  // The native share label now describes same-workspace ACL, not organization sharing.
  exact(editor, "{ key: 'share', icon: 'share', label: t('knowledgeEditor.sidebar.share') }", "{ key: 'share', label: '访问与协作', icon: 'share' }")

  const menu = 'src/views/knowledge/components/DocumentActionMenu.vue'
  context(menu)
  exact(menu, 'v-if="item.type === \'manual\'"', 'v-if="kbAcl.can(\'document.update\') && item.type === \'manual\'"')
  exact(menu, 'v-if="isParseInFlight"', 'v-if="kbAcl.canReparse() && isParseInFlight"', 2)
  exact(menu, '<t-popconfirm v-else theme="warning"', '<t-popconfirm v-else-if="kbAcl.canReparse()" theme="warning"')
  exact(menu, 'v-if="canMutateKnowledge" class="doc-action-menu-item" @click.stop="emit(\'move-folder\')"', 'v-if="kbAcl.can(\'document.update\')" class="doc-action-menu-item" @click.stop="emit(\'move-folder\')"')
  exact(menu, 'v-if="canMutateKnowledge" class="doc-action-menu-item" @click.stop="emit(\'move\')"', 'v-if="kbAcl.canManage()" class="doc-action-menu-item" @click.stop="emit(\'move\')"')
  exact(menu, '<t-popconfirm theme="warning"', '<t-popconfirm v-if="kbAcl.can(\'document.delete\')" theme="warning"')
  for (const name of ['DocumentCardView.vue', 'DocumentListView.vue']) {
    const path = 'src/views/knowledge/components/' + name
    context(path)
    const source = read(path)
    write(path, source.replaceAll('v-if="canEdit"', 'v-if="canEdit || canDownload || kbAcl.can(\'document.delete\')"'))
    if (name === 'DocumentCardView.vue') {
      exact(path, '<template v-if="canEdit || canDownload || kbAcl.can(\'document.delete\')">', '<template v-if="canEdit">')
      exact(path, 'v-if="canEdit && batchMode"', 'v-if="canMutateKnowledge && batchMode"')
    }
    // Tags remain document.update; the menu above handles each independent action.
  }
  const batch = 'src/views/knowledge/components/DocumentBatchBar.vue'
  context(batch)
  exact(batch, '<t-popconfirm theme="warning" :content="t(\'knowledgeBase.confirmBatchReparseDocument\'', '<t-popconfirm v-if="kbAcl.canReparse()" theme="warning" :content="t(\'knowledgeBase.confirmBatchReparseDocument\'')
  exact(batch, '<t-popconfirm theme="warning" :content="t(\'knowledgeBase.confirmBatchDeleteDocument\'', '<t-popconfirm v-if="kbAcl.can(\'document.delete\')" theme="warning" :content="t(\'knowledgeBase.confirmBatchDeleteDocument\'')
  exact(batch, '<t-button theme="default" variant="outline" size="small"\n            :disabled="count === 0', '<t-button v-if="kbAcl.can(\'document.update\')" theme="default" variant="outline" size="small"\n            :disabled="count === 0')

  const doc = 'src/components/doc-content.vue'
  exact(doc, 'import { marked } from "marked";', `import { marked } from "marked";\nimport { useKBPermissions } from '@/mindcreek/kb-permissions';\nimport KBChunkActions from '@/mindcreek/KBChunkActions.vue';`)
  regex(doc, /const canDeleteGeneratedQuestion = computed\(\(\) => \{[\s\S]*?\n\}\);/g, "const canDeleteGeneratedQuestion = computed(() => docAcl.can('chunk.update'));" )
  exact(doc, 'const canEditContent = canDeleteGeneratedQuestion;', "const canEditContent = computed(() => docAcl.can('document.update'));\nconst canEditChunks = computed(() => docAcl.can('chunk.update'));")
  exact(doc, 'const emit = defineEmits(["closeDoc", "getDoc", "questionDeleted", "summaryStateChange"]);', 'const docAcl = useKBPermissions(() => props.kbId || props.details?.knowledge_base_id);\nconst emit = defineEmits(["closeDoc", "getDoc", "questionDeleted", "summaryStateChange"]);')
  exact(doc, 'const canPreview = (): boolean => {', "const canPreview = (): boolean => {\n  if (!docAcl.can('document.download')) return false;")
  exact(doc, 'if (!props.details?.id || audioBlobUrl.value) return;', "if (!docAcl.can('document.download') || !props.details?.id || audioBlobUrl.value) return;")
  exact(doc, '<t-button v-if="canDownloadKB && (details.type === \'file\' || details.type === \'manual\')"', '<KBChunkActions v-if="details?.id" :kb-id="kbId || details.knowledge_base_id" :knowledge-id="details.id" :allowed-export="docAcl.can(\'chunk.export\')" />\n            <t-button v-if="docAcl.can(\'document.download\') && (details.type === \'file\' || details.type === \'manual\')"')
  // The chunk section is independent of document metadata/summary editing.
  const chunkStart = read(doc).indexOf('chunk.original.index_status')
  const docSource = read(doc)
  write(doc, docSource.slice(0, chunkStart) + docSource.slice(chunkStart).replaceAll('canEditContent', 'canEditChunks'))
  exact(doc, '<template v-if="canEditChunks">', '<KBChunkActions :kb-id="kbId || details.knowledge_base_id" :knowledge-id="details.id" :chunk-id="chunk.original.id" :allowed-delete="docAcl.can(\'chunk.delete\')" @changed="emit(\'getDoc\', pageInfo.current)" />\n                    <template v-if="canEditChunks">')
  exact(doc, 'const isTextFile = (fileType?: string): boolean => {', `watch(() => docAcl.state.value, (access) => {\n  if (!access?.visible) { metadataEditing.value = false; summaryEditing.value = false; editingChunkId.value = ''; viewMode.value = 'merged'; }\n  if (!docAcl.can('document.download')) { viewMode.value = 'merged'; if (audioBlobUrl.value) { URL.revokeObjectURL(audioBlobUrl.value); audioBlobUrl.value = ''; } }\n});\nconst isTextFile = (fileType?: string): boolean => {`)

  const faq = 'src/views/knowledge/components/FAQEntryManager.vue'
  context(faq)
  regex(faq, /const canEdit = computed\(\(\) => \{[\s\S]*?\n\}\)/g, "const canEdit = computed(() => kbAcl.can('document.update') && kbAcl.can('chunk.update'))")
  regex(faq, /const canManage = computed\(\(\) => \{[\s\S]*?\n\}\)/g, "const canManage = computed(() => kbAcl.can('document.delete'))")
  exact(faq, '<t-tooltip v-if="canManage" :content="$t(\'knowledgeBase.settings\')"', '<t-tooltip v-if="kbAcl.canManage()" :content="$t(\'knowledgeBase.settings\')"')
  exact(faq, '<div v-if="canEdit" class="tag-filter-panel__footer">', '<div v-if="kbAcl.canManage()" class="tag-filter-panel__footer">')
  exact(faq, 'if (!canEdit.value) return []', "if (!kbAcl.can('document.create')) return []")
  exact(faq, "    { content: t('knowledgeEditor.faqImport.importButton'), value: 'import', prefixIcon: () => h(TIcon, { name: 'upload', size: '16px' }) },", "    ...(canEdit.value ? [{ content: t('knowledgeEditor.faqImport.importButton'), value: 'import', prefixIcon: () => h(TIcon, { name: 'upload', size: '16px' }) }] : []),")
  exact(faq, '<t-dropdown :options="faqExportOptions"', '<t-dropdown v-if="kbAcl.can(\'chunk.export\')" :options="faqExportOptions"')
  exact(faq, '<t-popup v-if="canManage" v-model="entry.showMore"', '<t-popup v-if="canManage || canEdit" v-model="entry.showMore"')
  exact(faq, '<div class="popup-menu-item" @click.stop="handleMenuEdit(entry)">', '<div v-if="canEdit" class="popup-menu-item" @click.stop="handleMenuEdit(entry)">')
  exact(faq, '<div class="popup-menu-item delete" @click.stop="handleMenuDelete(entry)">', '<div v-if="canManage" class="popup-menu-item delete" @click.stop="handleMenuDelete(entry)">')
  exact(faq, '<t-radio-button value="replace">', '<t-radio-button v-if="kbAcl.can(\'document.delete\') && kbAcl.can(\'chunk.delete\')" value="replace">')

  const wiki = 'src/views/knowledge/wiki/WikiBrowser.vue'
  context(wiki)
  exact(wiki, 'v-if="props.canEdit" :content="$t(\'knowledgeEditor.wikiBrowser.newRootFolder\')"', 'v-if="kbAcl.canManage()" :content="$t(\'knowledgeEditor.wikiBrowser.newRootFolder\')"')
  exact(wiki, 'v-if="props.canEdit" :content="$t(\'knowledgeEditor.wikiBrowser.newPageBtn\')"', 'v-if="kbAcl.can(\'document.create\')" :content="$t(\'knowledgeEditor.wikiBrowser.newPageBtn\')"')
  exact(wiki, '<t-popconfirm v-if="props.canEdit" theme="danger"', '<t-popconfirm v-if="kbAcl.can(\'document.delete\')" theme="danger"')
  exact(wiki, '<WikiFolderActions v-if="item.folderId"', '<WikiFolderActions v-if="item.folderId && kbAcl.canManage()"')

  const list = 'src/views/knowledge/KnowledgeBaseList.vue'
  exact(list, '<div class="kb-list-main">', '<div class="kb-list-main">\n        <KBCreationRecovery v-if="authStore.hasRole(\'admin\')" />')
  exact(list, '<script setup lang="ts">', '<script setup lang="ts">\nimport KBCreationRecovery from \'@/mindcreek/KBCreationRecovery.vue\';')
  exact(list, '<script setup lang="ts">', `<script setup lang="ts">\nimport { fetchKBEffective } from '@/mindcreek/kb-permissions';\nimport { matchesKBFilter } from '@/mindcreek/kb-permissions-policy';\nimport { useKBListRefresh } from '@/mindcreek/kb-list-refresh';`)
  exact(list, 'const kbs = ref<KB[]>([])\nconst loading = ref(false)', `const aclFilter = ref('all')
const kbList = useKBListRefresh<KB, OrganizationSharedKnowledgeBaseItem>({
  identity: () => JSON.stringify([authStore.user?.id, authStore.effectiveTenantId, authStore.currentTenantRole, creatorFilter.value, spaceSelection.value]),
  access: fetchKBEffective,
  async load(force) {
    const scope = spaceSelection.value
    const isOrganization = !['all', 'mine', 'favorites', 'recents', 'shared', ''].includes(scope)
    const [rows, , , organization] = await Promise.all([
      chatResources.fetchKnowledgeBasesForList({ creator: creatorFilter.value }, force),
      orgStore.fetchSharedKnowledgeBases({ force }),
      orgStore.fetchOrganizations({ force }),
      isOrganization ? listOrganizationSharedKnowledgeBases(scope) : Promise.resolve({ success: true, data: [] }),
    ])
    if (!organization.success || !Array.isArray(organization.data)) throw new Error('Knowledge list unavailable')
    return {
      items: rows.map((kb: any) => ({ ...kb, updated_at: kb.updated_at ? formatStringDate(new Date(kb.updated_at)) : '', showMore: false, isProcessing: kb.is_processing || false, processing_count: kb.processing_count || 0 })),
      organizationItems: organization.data,
    }
  },
})
const { items: kbs, access: aclByKB, loading, ready: listReady, error: listError } = kbList
const canShowKBEmpty = computed(() => listReady.value && !loading.value && !listError.value)`)
  exact(list, 'const spaceKbsList = ref<OrganizationSharedKnowledgeBaseItem[]>([])\nconst spaceKbsLoading = ref(false)', 'const spaceKbsList = kbList.organizationItems\nconst spaceKbsLoading = loading')
  exact(list, 'const spaceCountByOrg = ref<Record<string, number>>({})', `const spaceCountByOrg = ref<Record<string, number>>({})
watch(() => orgStore.resourceCounts?.knowledge_bases?.by_organization, counts => {
  spaceCountByOrg.value = counts ? { ...counts } : {}
}, { immediate: true })`)
  regex(list, /const applyKbListData = \(data: any\[\]\) => \{[\s\S]*?\n\}\n\nconst fetchList = \(force = false\) => \{[\s\S]*?\n\}/g, `const fetchList = async (force = false) => {
  await kbList.refresh(force)
  const counts = orgStore.resourceCounts?.knowledge_bases?.by_organization
  if (counts) spaceCountByOrg.value = { ...counts }
}`)
  regex(list, /watch\(spaceSelection, \(val\) => \{[\s\S]*?\n\}, \{ immediate: true \}\)/g, `watch(spaceSelection, val => {
  if (val === 'shared') spaceSelection.value = 'all'
}, { immediate: true })`)
  exact(list, 'watch(creatorFilter, () => {\n  fetchList(true)\n})', '// Creator and scope changes are handled by the verified list context guard.')
  exact(list, '<div class="kb-list-main">', `<div class="kb-list-main">
        <div v-if="loading" data-testid="kb-list-loading" role="status" style="padding:16px 0"><t-loading size="small" /> 正在加载知识库并核验访问权限</div>
        <div v-if="listError" data-testid="kb-list-error" role="alert" style="padding:16px 0;display:flex;gap:16px;align-items:center"><span>{{ listError }}</span><t-button variant="outline" @click="kbList.retry()">重试</t-button></div>`)
  exact(list, '        <!-- 卡片网格：全部 / 收藏 / 最近', '        <template v-if="listReady && !loading">\n        <!-- 卡片网格：全部 / 收藏 / 最近')
  exact(list, '        <!-- 全部空状态：保留', '        </template>\n        <!-- 全部空状态：保留')
  // Every empty state (and the creation guide) waits for successful permission
  // checks. Failed/unfinished checks are never evidence of an empty workspace.
  write(list, read(list).replaceAll('&& !loading"', '&& canShowKBEmpty"'))
  exact(list, '<template v-if="listReady && canShowKBEmpty">', '<template v-if="listReady && !loading">')
  exact(list, 'spaceSelectionOrgId && !spaceKbsLoading && spaceKbsList.length === 0', 'spaceSelectionOrgId && canShowKBEmpty && spaceKbsList.length === 0')
  exact(list, '  if (loading.value) return false', '  if (!canShowKBEmpty.value) return false')
  regex(list, /function canManageKBCard\(kb: KB\): boolean \{[\s\S]*?\n\}/g, 'function canManageKBCard(kb: KB): boolean { return aclByKB.value[kb.id]?.visible === true && aclByKB.value[kb.id]?.manage === true }')
  exact(list, "return authStore.hasRole('contributor') && kb.isMine !== false", 'return canManageKBCard(kb)')
  exact(list, 'const filteredKnowledgeBases = computed(() => {', 'const nativeFilteredKnowledgeBases = computed(() => {')
  exact(list, 'const showKbListEmpty = computed(() => {', `const filteredKnowledgeBases = computed(() => nativeFilteredKnowledgeBases.value.filter(kb => matchesKBFilter(aclByKB.value[kb.id], aclFilter.value)))\nconst showKbListEmpty = computed(() => {`)
  exact(list, '<div class="kb-list-main">', `<div class="kb-list-main">\n        <div style="display:flex;gap:10px;padding:12px 0;align-items:center"><label>可见知识库 <select v-model="aclFilter" @change="spaceSelection = 'all'" aria-label="知识库权限筛选"><option value="all">{{ authStore.hasRole('admin') ? '全部知识库' : '全部可见' }}</option><option value="mine">我创建的</option><option value="shared">共享给我的</option><option value="workspace">空间公开</option></select></label><span style="color:var(--td-text-color-secondary)">新建默认仅创建者和空间管理者可见</span></div>`)

  // List/selector responses are server-filtered. Drop stale local selections on refresh.
  const selector = 'src/components/KnowledgeBaseSelector.vue'
  exact(selector, 'if (res?.data && Array.isArray(res.data)) knowledgeBases.value = res.data', `if (res?.data && Array.isArray(res.data)) {\n      knowledgeBases.value = res.data\n      const visible = new Set(res.data.map((kb: any) => kb.id))\n      settingsStore.selectKnowledgeBases(settingsStore.settings.selectedKnowledgeBases.filter((id: string) => visible.has(id)))\n    }`)
  const security = 'src/utils/security.ts'
  exact(security, "  | { status: 'failed' };", "  | { status: 'failed' }\n  | { status: 'unverified' };")
  exact(security, '          if (!resp.ok) {', "          if (!resp.ok) {\n            if (resp.status === 409) return { status: 'unverified' };")
  exact(security, "    if (result.status === 'failed') {", "    if (result.status === 'unverified') {\n      const notice = document.createElement('span');\n      notice.textContent = '此图片尚未完成权限归属验证，暂不可显示。';\n      notice.setAttribute('role', 'status'); img.replaceWith(notice); return;\n    }\n    if (result.status === 'failed') {")
}

export function applyKnowledgeKeyACL({ replaceExact: exact }) {
  const api = 'src/views/integrations/ApiIntegrationSettings.vue'
  exact(api, "import WorkspaceAdminDelegation from '@/mindcreek/WorkspaceAdminDelegation.vue'", "import WorkspaceAdminDelegation from '@/mindcreek/WorkspaceAdminDelegation.vue'\nimport KBCollaborationDelegation from '@/mindcreek/KBCollaborationDelegation.vue'\nimport { KB_OPERATIONS } from '@/mindcreek/kb-permissions-policy'")
  exact(api, '<WorkspaceAdminDelegation @ready="keyCreationReady = $event" />', '<WorkspaceAdminDelegation @ready="keyCreationReady = $event" />\n    <KBCollaborationDelegation />')
  for (const form of ['apiKeyForm', 'editingAPIKeyForm']) {
    exact(api, `const ${form} = reactive({`, `const ${form} = reactive({\n  knowledge_operations: [] as string[],\n  all_knowledge_bases: false,`)
    const anchor = `<t-select\n            v-model="${form}.knowledge_base_ids"`
    exact(api, anchor, `<div class="kb-machine-permissions"><p>知识内容操作（不勾选只允许原生能力范围内的阅读）。ingest 不会自动获得下列操作。</p><label v-for="op in KB_OPERATIONS" :key="op.value" style="display:inline-flex;gap:6px;margin:8px 16px 8px 0"><input type="checkbox" :value="op.value" v-model="${form}.knowledge_operations" />{{ op.label }}</label><p><label v-if="isWorkspaceOwner${form === 'editingAPIKeyForm' ? ' && !editingRestricted' : ''}"><input type="checkbox" v-model="${form}.all_knowledge_bases" /> 明确授权全部知识库，包括以后新建的私有库</label></p><p>未勾选全部知识库时，必须明确选择可访问的知识库；空间公开不会自动扩大密钥范围。</p></div>\n          ${anchor}`)
  }
  exact(api, '<div v-if="apiKeyKnowledgeScopeApplies" class="api-key-dialog-row">', '<div class="api-key-dialog-row">')
  exact(api, '<div v-if="editingKnowledgeScopeApplies" class="api-key-dialog-row">', '<div class="api-key-dialog-row">')
  exact(api, `:placeholder="$t('integrations.api.apiKeyKnowledgeScopePlaceholder')"`, 'placeholder="请选择知识库；未选择则不能访问知识库"', 2)
  exact(api, `{{ $t('integrations.api.editApiKeyScopeHint') }}`, '未选择知识库且未明确授权全部知识库时，密钥不能访问知识库。所有修改保存后立即生效。')
  exact(api, 'formatKeyKnowledgeScope(key.knowledge_base_ids)', 'formatKeyKnowledgeScope(key)')
  exact(api, 'function formatKeyKnowledgeScope(ids: readonly string[] | null | undefined) {\n  const normalizedIDs = normalizeAPIKeyKnowledgeBaseIDs(ids)\n  if (!normalizedIDs.length) return t(\'integrations.api.allKnowledgeBases\')', `function formatKeyKnowledgeScope(key: TenantAPIKey) {
  if (key.knowledge_mapped === false) return '知识范围尚未配置'
  if (key.all_knowledge_bases) return '全部知识库（包含以后新建的私有库）'
  const normalizedIDs = normalizeAPIKeyKnowledgeBaseIDs(key.knowledge_base_ids)
  if (!normalizedIDs.length) return '无知识库访问权限'`)
  exact(api, '    apiKeyForm.knowledge_base_ids = []', '    // Knowledge scope is independent of native capability mode.', 2)
  exact(api, '  if (!applies) editingAPIKeyForm.knowledge_base_ids = []', '  // Knowledge scope is independent of native capability mode.')
  exact(api, 'knowledge_base_ids: apiKeyKnowledgeScopeApplies.value ? apiKeyForm.knowledge_base_ids : [],', 'knowledge_base_ids: apiKeyForm.all_knowledge_bases ? [] : apiKeyForm.knowledge_base_ids,')
  exact(api, 'knowledge_base_ids: editingKnowledgeScopeApplies.value ? editingAPIKeyForm.knowledge_base_ids : [],', 'knowledge_base_ids: editingAPIKeyForm.all_knowledge_bases ? [] : editingAPIKeyForm.knowledge_base_ids,')
  exact(api, 'function openCreateAPIKeyDialog() {', 'function openCreateAPIKeyDialog() {\n  apiKeyForm.knowledge_operations = []; apiKeyForm.all_knowledge_bases = false')
  exact(api, '  editingAPIKey.value = key', '  editingAPIKey.value = key\n  editingAPIKeyForm.knowledge_operations = key.knowledge_operations || []\n  editingAPIKeyForm.all_knowledge_bases = key.all_knowledge_bases === true')
  exact(api, '      name: apiKeyForm.name.trim(),', '      name: apiKeyForm.name.trim(),\n      knowledge_operations: apiKeyForm.knowledge_operations,\n      all_knowledge_bases: isWorkspaceOwner.value && apiKeyForm.all_knowledge_bases,')
  exact(api, '      name: editingAPIKeyForm.name.trim(),', '      name: editingAPIKeyForm.name.trim(),\n      knowledge_operations: editingAPIKeyForm.knowledge_operations,\n      all_knowledge_bases: isWorkspaceOwner.value && !editingRestricted.value && editingAPIKeyForm.all_knowledge_bases,')
  exact(api, '<span class="api-key-name">{{ key.name }}</span>', '<span class="api-key-name">{{ key.name }}</span><small v-if="key.knowledge_mapped === false && !key.protected" class="scope-hint">知识访问尚未映射，请由 Owner 配置范围。</small>')
  const tenant = 'src/api/tenant/index.ts'
  exact(tenant, 'export interface TenantAPIKey {', 'export interface TenantAPIKey {\n  knowledge_mapped?: boolean\n  knowledge_operations?: string[]\n  all_knowledge_bases?: boolean')
  exact(tenant, 'export interface CreateTenantAPIKeyPayload {', 'export interface CreateTenantAPIKeyPayload {\n  knowledge_operations?: string[]\n  all_knowledge_bases?: boolean')
  exact(tenant, 'export interface UpdateTenantAPIKeyPayload {', 'export interface UpdateTenantAPIKeyPayload {\n  knowledge_operations?: string[]\n  all_knowledge_bases?: boolean')
}
