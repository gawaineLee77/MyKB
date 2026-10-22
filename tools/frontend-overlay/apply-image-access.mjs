// Scoped image rendering for the pinned frontend copy; no upstream edits.
export function applyScopedImageAccess({ replaceExact: exact, replaceRegex: regex }) {
  const bot = 'src/views/chat/components/botmsg.vue'
  const user = 'src/views/chat/components/usermsg.vue'
  for (const path of [bot, user]) {
    exact(path, '<script setup>\nimport', "<script setup>\nimport { messageImageAccess } from '@/mindcreek/message-image-access';\nimport")
  }
  exact(bot, "const showRequestInfo = computed(() => !!(props.session?.request_id || props.session?.id));", `const protectedImageAccess = computed(() => messageImageAccess(props.sessionId, props.session));
const hydrateMessageImages = async () => {
    const access = protectedImageAccess.value;
    if (access) await hydrateProtectedFileImages(parentMd.value, access);
};
watch(protectedImageAccess, () => { nextTick(hydrateMessageImages); });
const showRequestInfo = computed(() => !!(props.session?.request_id || props.session?.id));`)
  exact(bot, '        await hydrateProtectedFileImages(parentMd.value);', '        await hydrateMessageImages();', 2)
  exact(user, '    sessionId: {', "    messageId: { type: String, default: '' },\n    sessionId: {")
  exact(user, '    await hydrateProtectedFileImages(containerRef.value);', `    const access = messageImageAccess(props.sessionId, { id: props.messageId });
    if (access) await hydrateProtectedFileImages(containerRef.value, access);`)
  exact(user, 'watch(() => props.images, hydrateImages);', 'watch(() => [props.images, props.sessionId, props.messageId], hydrateImages);')
  exact('src/views/chat/index.vue', '                                :session-id="session_id">', '                                :session-id="session_id" :message-id="session.id || \'\'">')

  const security = 'src/utils/security.ts'
  exact(security, '  const withProviderImages = html.replace(', `  const withoutUnverifiedImages = html.replace(
    /<img\\b[^>]*\\bsrc=(["'])mindcreek-unverified-image\\1[^>]*>/gi,
    '<span role="status">此图片尚未完成权限归属验证，暂不可显示。</span>',
  );
  const withProviderImages = withoutUnverifiedImages.replace(`)
  // A Markdown render has not supplied its authorization scope yet. Reusing a
  // blob by source alone could briefly expose pixels from a previous identity
  // or from a different message/KB scope before the scoped fetch rejects it.
  regex(security, /  const cachedBlobURL = protectedFileBlobBySource\.get\(protectedSrc\);\n  if \(cachedBlobURL\) \{\n[\s\S]*?\n  \}/g, '')
  exact(security, '      protectedFileMissingSources.add(sourceURL);', '      // Missing in this scope does not imply missing in every KB/message scope.')
  exact(security, 'const PROTECTED_FILE_RETRY_COOLDOWN_MS = 5000;', `const PROTECTED_FILE_RETRY_COOLDOWN_MS = 5000;
let protectedImageEpoch = 0;
const protectedImageAccess = new WeakMap<HTMLImageElement, ProtectedFileAccessContext>();
function resetProtectedImageAuthorization(): void {
  protectedImageEpoch++;
  for (const blobURL of protectedFileBlobCache.values()) URL.revokeObjectURL(blobURL);
  protectedFileBlobCache.clear();
  protectedFileBlobBySource.clear();
  protectedFileMissingSources.clear();
  protectedFileFailureCache.clear();
  protectedFileInflight.clear();
  const images = Array.from(document.querySelectorAll<HTMLImageElement>('img[data-protected-src]'));
  for (const img of images) {
    img.src = PROVIDER_IMAGE_PLACEHOLDER;
    img.dataset.authHydrated = '0';
    img.hidden = false;
    if (img.nextElementSibling?.hasAttribute('data-protected-image-notice')) img.nextElementSibling.remove();
  }
  // Each image retains its own KB/message scope. Never rehydrate a whole page
  // using the currently selected KB or a tenant-wide default.
  for (const img of images) {
    const access = protectedImageAccess.get(img);
    if (access) void hydrateProtectedFileImages(img, access);
  }
}
if (typeof window !== 'undefined') {
  window.addEventListener('focus', resetProtectedImageAuthorization);
  window.addEventListener('mindcreek-kb-permissions-changed', resetProtectedImageAuthorization);
}`)
  exact(security, "  root.querySelectorAll<HTMLImageElement>('img[data-protected-src]').forEach((candidate) => {", "  const images = root instanceof HTMLImageElement ? [root] : root.querySelectorAll<HTMLImageElement>('img[data-protected-src]');\n  images.forEach((candidate) => {")
  exact(security, '  const images = root.querySelectorAll<HTMLImageElement>(', '  const images = root instanceof HTMLImageElement ? [root] : root.querySelectorAll<HTMLImageElement>(')
  exact(security, '  const resolvedAccess = resolveProtectedFileAccess(access);', '  const resolvedAccess = resolveProtectedFileAccess(access);\n  const epoch = protectedImageEpoch;')
  exact(security, '    const normalizedSourceURL = normalizeProtectedImageElement(img);', '    protectedImageAccess.set(img, resolvedAccess);\n    const normalizedSourceURL = normalizeProtectedImageElement(img);')
  exact(security, '    const { url: requestURL, headers } = request;', `    const { url: requestURL, headers } = request;
    const requestCacheKey = requestURL + '\\\\0' + JSON.stringify(headers);`)
  for (const name of ['protectedFileBlobCache', 'protectedFileFailureCache', 'protectedFileInflight']) {
    exact(security, `${name}.get(requestURL)`, `${name}.get(requestCacheKey)`)
  }
  exact(security, 'protectedFileFailureCache.set(requestURL, Date.now())', 'protectedFileFailureCache.set(requestCacheKey, Date.now())', 2)
  exact(security, 'protectedFileBlobCache.set(requestURL, blobURL)', 'protectedFileBlobCache.set(requestCacheKey, blobURL)')
  exact(security, 'protectedFileFailureCache.delete(requestURL)', 'protectedFileFailureCache.delete(requestCacheKey)')
  exact(security, 'protectedFileInflight.delete(requestURL)', 'protectedFileInflight.delete(requestCacheKey)')
  exact(security, 'protectedFileInflight.set(requestURL, loadTask)', 'protectedFileInflight.set(requestCacheKey, loadTask)')
  exact(security, "            method: 'GET',\n            headers,", "            method: 'GET',\n            cache: 'no-store',\n            headers,")
  exact(security, '          if (!resp.ok) {', "          if (epoch !== protectedImageEpoch) return { status: 'failed' };\n          if (!resp.ok) {")
  exact(security, '          const blobURL = URL.createObjectURL(blob);', "          if (epoch !== protectedImageEpoch) return { status: 'failed' };\n          const blobURL = URL.createObjectURL(blob);")
  exact(security, "          console.warn('[security] hydrateProtectedFileImages failed:', error);", "          if (epoch !== protectedImageEpoch) return { status: 'failed' };\n          console.warn('[security] hydrateProtectedFileImages failed:', error);")
  exact(security, '          protectedFileInflight.delete(requestCacheKey);', '          if (epoch === protectedImageEpoch) protectedFileInflight.delete(requestCacheKey);')
  exact(security, '    const result = await loadTask;', '    const result = await loadTask;\n    if (epoch !== protectedImageEpoch) return;')
  exact(security, "      notice.setAttribute('role', 'status'); img.replaceWith(notice); return;", `      notice.setAttribute('role', 'status');
      notice.setAttribute('data-protected-image-notice', '');
      if (img.nextElementSibling?.hasAttribute('data-protected-image-notice')) img.nextElementSibling.remove();
      img.hidden = true;
      img.insertAdjacentElement('afterend', notice); return;`)
}
