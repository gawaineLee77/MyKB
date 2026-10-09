<template>
  <t-button size="small" variant="outline" shape="round" :loading="busy" :disabled="busy"
    title="导出 Word（可编辑表格和公式）" aria-label="导出 Word" @click.stop="download">
    <t-icon name="download" /><span>Word</span>
  </t-button>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { CitationKnowledgeRef } from '../utils/citationMarkdown'
const props = defineProps<{ content: string; title?: string; references?: CitationKnowledgeRef[] }>()
const busy = ref(false)
async function download() {
  if (busy.value) return
  busy.value = true
  try {
    const { downloadAnswerWord } = await import('./word-export')
    const result = await downloadAnswerWord(props)
    if (result.warnings.length) MessagePlugin.warning({ content: 'Word 已导出。' + result.warnings.join(' '), duration: 10000 })
    else MessagePlugin.success('Word 已导出，表格和支持的公式可编辑。')
  } catch (error) {
    MessagePlugin.error({ content: error instanceof Error ? error.message : 'Word 导出失败，请重试。', duration: 12000 })
  } finally { busy.value = false }
}
</script>
