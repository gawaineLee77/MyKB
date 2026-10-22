<template><div v-if="acl.state.value?.visible && !acl.canManage()" class="kb-effective-notice" role="status">你可阅读解析内容并用于问答。<span v-if="labels.length">已获操作：{{ labels.join('、') }}。</span><span v-else>当前未获写入、原文件下载或分块导出权限。</span></div></template>
<script setup lang="ts">
import { computed } from 'vue'
import { useKBPermissionsContext } from './kb-permissions'
import { KB_OPERATIONS } from './kb-permissions-policy'
const acl = useKBPermissionsContext()
const labels = computed(() => KB_OPERATIONS.filter(op => acl.can(op.value)).map(op => op.label))
</script>
<style scoped>.kb-effective-notice{padding:10px 24px;background:var(--td-bg-color-secondarycontainer);color:var(--td-text-color-secondary);font-size:13px;line-height:1.6}</style>
