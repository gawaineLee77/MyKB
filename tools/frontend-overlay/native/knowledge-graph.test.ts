import test from 'node:test'
import assert from 'node:assert/strict'
import { canBrowseGraph, edgeGeometry, emptyGraph, layoutGraph, mergeGraph, type KnowledgeGraph } from './knowledge-graph'

test('graph visibility follows three roles, never platform-admin alone', () => {
  for (const role of ['owner', 'admin', 'contributor']) assert.equal(canBrowseGraph(role), true)
  for (const role of ['viewer', 'system_admin', '', undefined]) assert.equal(canBrowseGraph(role), false)
})
test('expansion keeps homonyms distinct, deduplicates IDs and bounds client memory', () => {
  const graph: KnowledgeGraph = { ...emptyGraph(), nodes: Array.from({ length: 300 }, (_, i) => ({ id: String(i), name: '同名实体', knowledge_id: 'doc', attributes: [] })), edges: [{ id: 'out', source: '0', target: '299', type: 'USES' }] }
  const result = mergeGraph(graph, graph)
  assert.equal(result.nodes.length, 240); assert.equal(result.truncated, true); assert.deepEqual(result.edges, [])
  assert.equal(new Set(result.nodes.map(n => n.id)).size, 240)
})
test('layout supports empty, isolated and connected entities with finite positions', () => {
  assert.deepEqual(layoutGraph(emptyGraph()), [])
  const graph: KnowledgeGraph = { ...emptyGraph(), nodes: ['a','b','c'].map(id => ({ id, name: id, knowledge_id: 'doc', attributes: [] })), edges: [{ id: 'r', source: 'a', target: 'b', type: 'USES' }] }
  const result = layoutGraph(graph)
  assert.equal(result.length, 3)
  assert.ok(result.every(n => Number.isFinite(n.x) && Number.isFinite(n.y) && n.x >= 55 && n.x <= 945))
})

test('self relations and reciprocal parallel edges have distinct visible curves', () => {
  const nodes = new Map(['a','b'].map((id,i) => [id, { id, name:id, knowledge_id:'doc', attributes:[], x:100+i*200,y:200 }]))
  const edges = [{id:'1',source:'a',target:'b',type:'USES'}, {id:'2',source:'b',target:'a',type:'KNOWS'}]
  assert.notEqual(edgeGeometry(edges[0]!,nodes,edges).y,edgeGeometry(edges[1]!,nodes,edges).y)
  assert.ok(edgeGeometry({id:'self',source:'a',target:'a',type:'LOOP'},nodes,[]).path.includes(' C '))
})
