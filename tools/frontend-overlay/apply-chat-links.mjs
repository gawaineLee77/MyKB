// Keep chat's existing _blank/noopener policy through DOMPurify's profile reset.
// Apply only to the isolated product copy, never the pinned upstream checkout.
export function applyChatLinks({ replaceExact: exact }) {
  exact('src/utils/markdownDomPurify.ts',
    'export const markdownDomPurifyConfig = {',
    `export const markdownDomPurifyConfig = {
  // USE_PROFILES replaces ALLOWED_ATTR; retain the target set by chat's hook.
  ADD_ATTR: ['target'],`)
}
