import test from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { createAnswerWord, wordFilename } from './word-export.ts'

test('Word exports real editable tables, text, numbering and code', async () => {
  const result = await createAnswerWord({ title: '测试报告', content: '# 结果\n\n**结论**：通过。\n\n| 项目 | 数值 |\n| --- | ---: |\n| A | 10 |\n| B | 20 |\n\n3. 第一项\n4. 第二项\n\n```ts\nconst x = "<xml>";\n```' })
  const zip = await JSZip.loadAsync(result.bytes), document = await zip.file('word/document.xml')!.async('string')
  assert.equal(result.tables, 1)
  assert.equal(result.equations, 0)
  assert.equal((document.match(/<w:tc>/g) || []).length, 6)
  assert.match(document, /<w:tblHeader\/>/)
  assert.match(document, /&lt;xml&gt;/)
  assert.match(document, /<w:b\/>/)
  assert.match(await zip.file('word/numbering.xml')!.async('string'), /w:start w:val="3"/)
  assert.ok(zip.file('[Content_Types].xml'))
  assert.doesNotMatch(document, /altChunk|documentProtection|w:drawing/)
})

test('export never fetches images; executable links and HTML remain inert', async () => {
  const result = await createAnswerWord({ content: '[危险](javascript:alert)\n\n![示意图](https://example.invalid/private.png)\n\n<script>alert(1)</script>' })
  const zip = await JSZip.loadAsync(result.bytes)
  assert.match(await zip.file('word/document.xml')!.async('string'), /&lt;script&gt;/)
  assert.doesNotMatch(await zip.file('word/_rels/document.xml.rels')!.async('string'), /javascript:|private.png/)
  assert.equal(result.warnings.length, 3)
})

test('authorized citation metadata accompanies answer, without an additional retrieval', async () => {
  const result = await createAnswerWord({ content: '结论。<kb doc="指南" chunk_id="1"/>', references: [{ id: 'chunk', knowledge_title: '指南', chunk_index: 3 }] })
  const zip = await JSZip.loadAsync(result.bytes), doc = await zip.file('word/document.xml')!.async('string')
  assert.match(doc, /superscript/)
  assert.match(doc, /参考来源/)
  assert.match(doc, /指南 · 分块 3/)
})

test('invalid input and unsupported formulas stop export', async () => {
  await assert.rejects(createAnswerWord({ content: '' }), /回答为空/)
  await assert.rejects(createAnswerWord({ content: 'x'.repeat(200001) }), /回答过长/)
  await assert.rejects(createAnswerWord({ content: '$$x' }), /结束标记/)
  await assert.rejects(createAnswerWord({ content: '$\\unsupported{x}$' }), /第 1 个公式/)
  await assert.rejects(createAnswerWord({ content: '$\\phantom{x}$' }), /不支持/)
})

test('math markers inside code and currency do not become equations', async () => {
  const result = await createAnswerWord({ content: '费用 $20 和 $30。代码 `$x^2$`\n\n```tex\n\\[x^2\\]\n```' })
  assert.equal(result.equations, 0)
})

test('filename is safe and bounded', () => {
  assert.equal(wordFilename('../a:b/*?'), 'MindCreek-.. a b.docx')
  assert.equal(wordFilename(''), 'MindCreek-回答.docx')
  assert.ok(wordFilename('长'.repeat(500)).length < 100)
})
