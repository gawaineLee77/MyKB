<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { get } from '@/utils/request'
import { useAuthStore } from '@/stores/auth'
import { canBrowseGraph, edgeGeometry, emptyGraph, layoutGraph, mergeGraph, type GraphNode, type GraphEdge, type KnowledgeGraph, type PlacedNode } from './knowledge-graph'

const props = defineProps<{ knowledgeBaseId: string }>()
const emit = defineEmits<{ 'open-source-doc': [id: string] }>()
const auth = useAuthStore(), graph = ref(emptyGraph()), placed = ref<PlacedNode[]>([])
const query = ref(''), relation = ref(''), busy = ref(false), problem = ref(''), selectedNode = ref<GraphNode>(), selectedEdge = ref<GraphEdge>()
const zoom = ref(1), pan = ref({ x: 0, y: 0 }), svg = ref<SVGSVGElement>()
const byID = computed(() => new Map(placed.value.map(n => [n.id, n])))
const edgeShapes = computed(() => new Map(graph.value.edges.map(e => [e.id, edgeGeometry(e, byID.value, graph.value.edges)])))
const relations = ref<string[]>([])
const permitted = computed(() => canBrowseGraph(auth.currentTenantRole))
let revision = 0
const messages: Record<string, string> = {
  'graph.dependencies_unavailable': '图谱服务尚未启用或未配置连接，请联系管理员。',
  'graph.unavailable': '暂时无法连接图谱服务，请稍后重试。',
  'graph.source_unavailable': '无法核验来源文档，请稍后重试。',
  'graph.role_denied': '当前角色无权查看知识图谱。',
  'workspace.denied': '当前空间无权查看此知识图谱。',
}
function resetView() { zoom.value = 1; pan.value = { x: 0, y: 0 } }
async function load(nodeID = '') {
  const ticket = ++revision
  if (!permitted.value || !props.knowledgeBaseId) { graph.value = emptyGraph(); placed.value = []; return }
  busy.value = true; problem.value = ''
  const prior = graph.value
  // Clear stale data while authorizing a new request, including expansion.
  graph.value = emptyGraph(); placed.value = []; selectedNode.value = undefined; selectedEdge.value = undefined
  try {
    const params = new URLSearchParams({ limit: nodeID ? '60' : '80' })
    if (nodeID) params.set('node_id', nodeID)
    else if (query.value.trim()) params.set('q', query.value.trim())
    if (relation.value) params.set('relation', relation.value)
    const response = await get(`/api/v1/mindcreek/graph/knowledge-bases/${encodeURIComponent(props.knowledgeBaseId)}?${params}`)
    if (ticket !== revision) return
    const next = response.data as KnowledgeGraph
    graph.value = nodeID && next.state === 'ready' ? mergeGraph(prior, next) : next
    placed.value = layoutGraph(graph.value)
    relations.value = [...new Set([...relations.value, ...next.edges.map(e => e.type)])].sort()
    resetView()
  } catch (error: any) {
    if (ticket !== revision) return
    const code = error?.error?.code || error?.response?.data?.error?.code
    problem.value = messages[code] || '图谱加载失败或访问权限已失效，请重试。'
  } finally { if (ticket === revision) busy.value = false }
}
watch(() => `${props.knowledgeBaseId}:${auth.effectiveTenantId}:${auth.currentTenantRole}`, () => {
  query.value = ''; relation.value = ''; relations.value = []; void load()
}, { immediate: true })
onUnmounted(() => { revision++ })
const edgeNodes = computed(() => selectedEdge.value ? [byID.value.get(selectedEdge.value.source), byID.value.get(selectedEdge.value.target)].filter((n): n is PlacedNode => !!n) : [])
const sources = computed(() => [...new Set(selectedNode.value ? [selectedNode.value.knowledge_id] : edgeNodes.value.map(n => n.knowledge_id))])
function selectNode(n: GraphNode) { selectedNode.value = n; selectedEdge.value = undefined }
function selectEdge(e: GraphEdge) { selectedEdge.value = e; selectedNode.value = undefined }
function point(event: PointerEvent) {
  const matrix = svg.value?.getScreenCTM()?.inverse()
  return matrix ? new DOMPoint(event.clientX, event.clientY).matrixTransform(matrix) : new DOMPoint()
}
let drag: { id?: string; x: number; y: number } | undefined
function start(event: PointerEvent, id?: string) {
  if (event.button !== 0) return
  const p = point(event); drag = { id, x: p.x, y: p.y }; svg.value?.setPointerCapture(event.pointerId)
}
function move(event: PointerEvent) {
  if (!drag) return
  const p = point(event), dx = p.x - drag.x, dy = p.y - drag.y
  const n = drag.id ? byID.value.get(drag.id) : undefined
  if (n) { n.x += dx / zoom.value; n.y += dy / zoom.value }
  else { pan.value.x += dx; pan.value.y += dy }
  drag.x = p.x; drag.y = p.y
}
function scale(factor: number) { zoom.value = Math.max(0.4, Math.min(3, zoom.value * factor)) }
</script>

<template>
  <section v-if="permitted" class="knowledge-graph" aria-label="知识图谱">
    <div class="graph-heading"><div><h3>知识图谱</h3><p>浏览文档中提取的实体与关系，选择节点可查看来源并展开邻居。</p></div><span class="readonly-badge">只读浏览</span></div>
    <form class="graph-toolbar" @submit.prevent="load()">
      <input v-model="query" aria-label="搜索实体" placeholder="搜索实体名称" maxlength="100" />
      <button type="submit" :disabled="busy">搜索</button>
      <label>关系类型 <input v-model="relation" list="graph-relations" placeholder="全部关系" maxlength="256" aria-label="关系类型" /></label>
      <datalist id="graph-relations"><option v-for="r in relations" :key="r" :value="r" /></datalist>
      <button type="button" :disabled="busy" @click="load()">应用筛选</button>
      <button type="button" :disabled="busy" @click="query = ''; relation = ''; load()">重置</button>
    </form>
    <div v-if="busy" class="graph-empty" role="status"><t-loading /><p>正在加载知识图谱…</p></div>
    <div v-else-if="problem" class="graph-empty" role="alert"><p>{{ problem }}</p><button @click="load()">重新加载</button></div>
    <div v-else-if="graph.state === 'disabled'" class="graph-empty"><h4>此知识库尚未启用知识图谱</h4><p>在知识库设置中开启实体关系提取，处理文档后即可查看。</p></div>
    <div v-else-if="!graph.nodes.length" class="graph-empty"><h4>{{ query || relation ? '没有匹配的实体或关系' : '暂无已提取的实体' }}</h4><p>请确认文档已完成图谱提取，或调整搜索条件。</p></div>
    <template v-else>
      <div class="graph-summary"><span>已加载 {{ graph.nodes.length }} 个实体 · {{ graph.edges.length }} 条关系</span><span v-if="graph.truncated">当前为部分图谱，可搜索实体或展开邻居；最多展示 240 个实体。</span></div>
      <div class="graph-workspace">
        <div class="graph-canvas">
          <div class="graph-controls"><button aria-label="放大图谱" @click="scale(1.2)">＋</button><button aria-label="缩小图谱" @click="scale(1 / 1.2)">－</button><button @click="resetView">复位视图</button></div>
          <svg ref="svg" viewBox="0 0 1000 620" aria-label="实体关系图" @pointerdown="start($event)" @pointermove="move" @pointerup="drag = undefined" @pointercancel="drag = undefined" @wheel.prevent="scale($event.deltaY < 0 ? 1.1 : 1 / 1.1)">
            <defs><marker id="kg-arrow" viewBox="0 0 10 10" refX="28" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#819c95" /></marker></defs>
            <g :transform="`translate(${pan.x} ${pan.y}) translate(500 310) scale(${zoom}) translate(-500 -310)`">
              <g v-for="e in graph.edges" :key="e.id" class="graph-edge" tabindex="0" role="button" :aria-label="`关系：${byID.get(e.source)?.name} ${e.type} ${byID.get(e.target)?.name}`" @pointerdown.stop @click="selectEdge(e)" @keydown.enter="selectEdge(e)">
                <path :d="edgeShapes.get(e.id)?.path" :class="{ chosen: selectedEdge?.id === e.id }" fill="none" stroke="#94b2a8" :stroke-width="selectedEdge?.id === e.id ? 4 : 2.5" marker-end="url(#kg-arrow)" />
                <text :x="edgeShapes.get(e.id)?.x" :y="edgeShapes.get(e.id)?.y">{{ e.type }}</text>
              </g>
              <g v-for="n in placed" :key="n.id" class="graph-node" :transform="`translate(${n.x} ${n.y})`" tabindex="0" role="button" :aria-label="`实体：${n.name}`" @pointerdown.stop="start($event, n.id); selectNode(n)" @click="selectNode(n)" @keydown.enter="selectNode(n)">
                <circle r="22" :class="{ chosen: selectedNode?.id === n.id }" /><text y="43">{{ n.name.length > 18 ? n.name.slice(0, 18) + '…' : n.name }}</text><title>{{ n.name }}</title>
              </g>
            </g>
          </svg>
        </div>
        <aside class="graph-details" aria-label="图谱详情">
          <template v-if="selectedNode || selectedEdge">
            <span class="detail-kind">{{ selectedNode ? '实体详情' : '关系详情' }}</span>
            <h4>{{ selectedNode?.name || selectedEdge?.type }}</h4>
            <p v-if="selectedEdge">{{ edgeNodes[0]?.name }} → {{ edgeNodes[1]?.name }}</p>
            <template v-if="selectedNode"><h5>提取属性</h5><ul v-if="selectedNode.attributes.length"><li v-for="(a, i) in selectedNode.attributes" :key="i">{{ a }}</li></ul><p v-else>未提取额外属性</p><button :disabled="busy" @click="load(selectedNode.id)">展开相邻实体</button></template>
            <h5>来源文档</h5><button v-for="(id, i) in sources" :key="id" class="source-link" @click="emit('open-source-doc', id)">查看来源文档{{ sources.length > 1 ? ` ${i + 1}` : '' }} ↗</button>
          </template>
          <template v-else><h4>探索知识之间的联系</h4><p>点击实体或连线查看详情。拖动节点整理布局，滚轮缩放，拖动画布平移。</p><h5>当前实体</h5><button v-for="n in graph.nodes" :key="n.id" class="entity-list-item" @click="selectNode(n)">{{ n.name }}</button></template>
        </aside>
      </div>
    </template>
  </section>
</template>

<style scoped>
.graph-node:focus:not(:focus-visible),.graph-edge:focus:not(:focus-visible){outline:none}
.knowledge-graph{margin:12px 24px 24px;color:#253c36}.graph-heading{display:flex;justify-content:space-between;align-items:center;margin:8px 0 20px}.graph-heading h3{font-size:20px;margin:0 0 8px}.graph-heading p,.graph-details p{font-size:13px;color:#697c76;line-height:1.7;margin:0}.readonly-badge{font-size:12px;background:#edf5f1;color:#37775f;padding:6px 12px;border-radius:20px;white-space:nowrap}.graph-toolbar{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.graph-toolbar label{font-size:13px;display:flex;align-items:center;gap:10px;margin-left:8px}.graph-toolbar input{width:175px;border:1px solid #d8e2dd;border-radius:6px;padding:9px 12px;background:white;color:#253c36}.knowledge-graph button{cursor:pointer;border:1px solid #d4e1da;border-radius:6px;background:white;padding:8px 12px;font:inherit;font-size:13px;color:#276e53}.knowledge-graph button:hover{background:#edf6f0}.knowledge-graph button:disabled{opacity:.5;cursor:wait}.graph-summary{display:flex;justify-content:space-between;gap:16px;font-size:12px;color:#6d7d76;padding:18px 0 10px}.graph-workspace{display:grid;grid-template-columns:minmax(0,1fr) 260px;min-height:480px;border:1px solid #e0e8e3;border-radius:12px;overflow:hidden}.graph-canvas{position:relative;background-color:#fafcfb;background-image:radial-gradient(#dfe9e3 1px,transparent 1px);background-size:20px 20px;min-width:0}.graph-controls{position:absolute;top:14px;left:14px;z-index:1;display:flex;gap:6px}.graph-canvas svg{display:block;width:100%;height:100%;min-height:480px;touch-action:none}.graph-node{cursor:grab}.graph-node circle{fill:#48a580;stroke:#fff;stroke-width:3}.graph-node circle.chosen{fill:#236f52;stroke:#a5d9bf;stroke-width:6}.graph-node text{font-size:13px;fill:#304d40;text-anchor:middle;paint-order:stroke;stroke:#fafcfb;stroke-width:4px;stroke-linejoin:round}.graph-edge{cursor:pointer}.graph-edge line{stroke:#94b2a8;stroke-width:2.5}.graph-edge line.chosen{stroke:#28734e;stroke-width:4}.graph-edge text{text-anchor:middle;font-size:11px;fill:#68877a;paint-order:stroke;stroke:#fafcfb;stroke-width:4px}.graph-details{border-left:1px solid #e0e8e3;padding:22px;background:#fff;overflow:auto;max-height:650px;overflow-wrap:anywhere}.graph-details h4{font-size:16px;margin:12px 0}.graph-details h5{font-size:12px;color:#78877f;margin:24px 0 10px}.graph-details ul{padding-left:18px;font-size:13px;line-height:1.8}.detail-kind{font-size:11px;color:#6d8879}.graph-details .source-link,.graph-details .entity-list-item{display:block;border:0;text-align:left;width:100%;padding:8px 0}.graph-empty{min-height:380px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px;color:#73867a}.graph-empty h4,.graph-empty p{margin:0;font-size:14px}@media(max-width:900px){.graph-workspace{grid-template-columns:1fr}.graph-details{border-left:0;border-top:1px solid #e0e8e3;max-height:240px}.graph-heading{gap:12px}.graph-summary{flex-direction:column}}
</style>
