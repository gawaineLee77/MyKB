export type WorkspaceRole = 'owner' | 'admin' | 'contributor' | 'viewer' | '' | null
export const workspaceHome = (role: string | null) => ['owner', 'admin', 'contributor'].includes(role || '')
  ? '/platform/knowledge-bases' : '/platform/creatChat'

export const SUPPORTED_AGENT_TOOLS = new Set([
  'thinking', 'todo_write', 'knowledge_search', 'grep_chunks', 'list_knowledge_chunks',
  'query_knowledge_graph', 'get_document_info', 'database_query', 'data_analysis', 'data_schema', 'wiki_search', 'wiki_read_page',
  'wiki_read_source_doc', 'wiki_flag_issue', 'wiki_write_page', 'wiki_replace_text',
  'wiki_rename_page', 'wiki_delete_page', 'wiki_read_issue', 'wiki_update_issue',
])

export function agentConfigurationProblem(config: Record<string, any>): string {
  if (config.memory_enabled) return '此安装未启用记忆。请关闭已有配置的记忆后保存。'
  const tools = config.allowed_tools || []
  const unsupported = tools.filter((name: string) => !SUPPORTED_AGENT_TOOLS.has(name))
  if (unsupported.length) return `请移除当前不支持的工具：${unsupported.join('、')}`
  if ((config.agent_mode === 'smart-reasoning' || config.max_iterations > 1 || config.max_iterations < 0) && !tools.length) {
    return '推理模式需要明确选择知识检索或 Wiki 工具。'
  }
  return ''
}

export function destinationForRole(role: string | null, path: string, section?: unknown): string | undefined {
  if (role !== 'viewer') return undefined
  if (path === '/platform/settings' && (!section || ['general', 'userprofile'].includes(String(section)))) return undefined
  if (path.startsWith('/platform/') && !/^\/platform\/(creatChat|chat\/[^/]+|retired)$/.test(path)) return '/platform/creatChat'
  if (path === '/knowledgeBase') return '/platform/creatChat'
  return undefined
}
