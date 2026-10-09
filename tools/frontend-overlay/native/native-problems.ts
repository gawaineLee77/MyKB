export const nativeProblems: Record<string, string> = {
  'feature.retired': '此功能已退役，请使用原生知识库或对话入口。',
  'scope.empty': '当前没有可用的知识范围，请切换空间或联系所有者。',
  'session.scope_empty': '当前没有可搜索的已授权会话。',
  'history.vector_scope_unavailable': '当前仅支持历史关键词搜索。',
  'history.aggregate_scope_unavailable': '当前不提供全空间历史统计。',
  'agent.tools_scope_unavailable': '此智能体需要配置明确的知识库或 Wiki 工具列表，请联系创建者。',
  'agent.source_ambiguous': '此智能体存在同 ID 的空间来源冲突，请选择唯一的自定义智能体。',
  'graph.dependencies_unavailable': '图谱依赖尚未就绪，请先完成服务配置。',
  'graph.unavailable': '暂时无法连接图谱服务，请稍后重试。',
  'graph.source_unavailable': '无法核验图谱来源文档，请稍后重试。',
  'graph.role_denied': '当前角色无权查看知识图谱。',
  'graph.invalid_query': '图谱查询条件无效，请调整后重试。',
  'member.account_not_registered': '该员工尚未通过企业登录开户，或邮箱映射存在冲突。',
  'workspace.owner_required': '此操作仅允许当前空间所有者执行。',
}
export function nativeProblem(data: any, fallback: string): string {
  const code = data?.error?.code || data?.code
  return nativeProblems[code] || data?.error?.message || data?.message || fallback
}
