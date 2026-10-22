<template>
  <section class="service-branding" data-testid="service-branding">
    <header><h2>服务名称与 Logo</h2><p>整个服务统一使用此名称与图标，适用于所有空间及登录页面。只有安装时配置的本地管理员可以修改。</p></header>
    <p v-if="error" class="problem" role="alert">{{ error }}</p>
    <p v-if="saved" class="success" role="status">已保存，所有空间的服务外观已更新。</p>
    <form @submit.prevent="save">
      <label class="name-label" for="service-name">服务名称</label>
      <input id="service-name" v-model="name" autocomplete="off" :disabled="busy || loading" placeholder="例如：小纳" />
      <p class="hint">可随时修改，支持 1–64 个字符。</p>
      <div class="logo-editor">
        <div class="preview"><img :src="preview" alt="服务 Logo 预览" /></div>
        <div><label class="upload-button">上传 Logo<input data-testid="brand-logo-file" type="file" accept="image/png,image/jpeg,image/webp" :disabled="busy || loading" @change="chooseLogo" /></label>
          <button type="button" class="remove-button" :disabled="busy || loading || (!currentLogo && !logo)" @click="removeLogo">恢复默认图标</button>
          <p class="hint">PNG、JPEG 或 WebP，最大 2 MiB。保留比例与透明背景。</p></div>
      </div>
      <div class="brand-preview" aria-label="服务外观预览"><img :src="preview" alt="" /><span>{{ name.trim() || '服务名称' }}</span></div>
      <footer><t-button type="submit" theme="primary" :loading="busy" :disabled="loading || conflict">保存服务外观</t-button><t-button variant="outline" :disabled="busy" @click="load">重新载入</t-button><span v-if="revision">版本 {{ revision }}</span></footer>
    </form>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import fallbackLogo from '@/assets/img/mindcreek-mark.png'
import { refreshBranding, saveBranding } from './branding'
import { brandingConflict, brandingProblem } from './branding-policy'
import { normalizedImageDataURL } from './image-upload'
const name = ref(''), currentLogo = ref(''), revision = ref(0), logo = ref<string | null | undefined>(undefined)
const loading = ref(true), busy = ref(false), conflict = ref(false), error = ref(''), saved = ref(false)
const preview = computed(() => logo.value === null ? fallbackLogo : logo.value || currentLogo.value || fallbackLogo)
async function load() {
  loading.value = true; error.value = ''; saved.value = false
  try { const value = await refreshBranding(); name.value = value.name; currentLogo.value = value.logo_url; revision.value = value.revision; logo.value = undefined; conflict.value = false }
  catch (value) { error.value = brandingProblem(value) }
  finally { loading.value = false }
}
async function chooseLogo(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]
  input.value = ''; if (!file) return
  error.value = ''; saved.value = false; busy.value = true
  try { logo.value = await normalizedImageDataURL(file) } catch (value) { error.value = brandingProblem(value) } finally { busy.value = false }
}
function removeLogo() { logo.value = null; saved.value = false }
async function save() {
  if (busy.value || conflict.value) return
  busy.value = true; error.value = ''; saved.value = false
  try { const value = await saveBranding(revision.value, name.value, logo.value); name.value = value.name; currentLogo.value = value.logo_url; revision.value = value.revision; logo.value = undefined; saved.value = true }
  catch (value) { error.value = brandingProblem(value); conflict.value = brandingConflict(value) }
  finally { busy.value = false }
}
onMounted(load)
</script>
<style scoped>
.service-branding{max-width:720px;color:var(--td-text-color-primary)}header h2{font-size:20px;margin:0 0 12px}header p,.hint{color:var(--td-text-color-secondary);line-height:1.7}header p{margin-bottom:28px;max-width:620px}.name-label{display:block;font-weight:600;margin-bottom:10px}#service-name{display:block;width:100%;max-width:460px;border:1px solid var(--td-border-level-2-color);border-radius:7px;padding:11px 12px;background:var(--td-bg-color-container);font:inherit;color:inherit;box-sizing:border-box}.hint{font-size:12px;margin:8px 0}.logo-editor{display:flex;gap:20px;align-items:center;margin:28px 0}.preview{height:96px;width:96px;flex:none;display:grid;place-items:center;border:1px solid var(--td-border-level-1-color);border-radius:16px;background:var(--td-bg-color-secondarycontainer)}.preview img{width:72px;height:72px;object-fit:contain}.upload-button{position:relative;overflow:hidden;display:inline-block;border:1px solid var(--td-border-level-2-color);padding:8px 14px;border-radius:6px;font-size:14px;cursor:pointer}.upload-button input{position:absolute;inset:0;opacity:0;width:100%;cursor:pointer}.remove-button{border:0;background:transparent;color:var(--td-text-color-secondary);cursor:pointer;padding:8px 12px}.remove-button:disabled{opacity:.45;cursor:default}.brand-preview{display:flex;align-items:center;gap:12px;max-width:420px;padding:22px;background:var(--td-bg-color-secondarycontainer);border:1px solid var(--td-border-level-1-color);border-radius:12px;margin-bottom:28px}.brand-preview img{width:40px;height:40px;object-fit:contain}.brand-preview span{font-size:20px;font-weight:600;overflow-wrap:anywhere;min-width:0}.problem{color:var(--td-error-color);line-height:1.6}.success{color:var(--td-success-color);line-height:1.6}footer{display:flex;gap:12px;align-items:center}footer span{font-size:12px;color:var(--td-text-color-placeholder)}@media(max-width:600px){.logo-editor{align-items:flex-start;gap:12px}.preview{width:76px;height:76px}.preview img{width:54px;height:54px}footer{flex-wrap:wrap}}
</style>
