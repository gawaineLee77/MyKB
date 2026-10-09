import { Marked, type Token, type Tokens, type TokenizerExtension } from 'marked'
import JSZip from 'jszip'
import { latexToWord, xml } from './word-math'
import { resolveCitationChunkId, type CitationKnowledgeRef } from '../utils/citationMarkdown'

export interface WordAnswer { content: string; title?: string; references?: CitationKnowledgeRef[] }
export interface WordExport { bytes: Uint8Array; filename: string; warnings: string[]; equations: number; tables: number }
type MathToken = Token & { type: 'wordMath'; text: string; display: boolean }
const W = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'
const M = 'http://schemas.openxmlformats.org/officeDocument/2006/math'
const R = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
const xmlHeader = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
const para = (runs: string, props = '') => `<w:p>${props ? `<w:pPr>${props}</w:pPr>` : ''}${runs}</w:p>`
const run = (s: string, props = '') => `<w:r>${props ? `<w:rPr>${props}</w:rPr>` : ''}${s.split('\n').map(line => `<w:t xml:space="preserve">${xml(line)}</w:t>`).join('<w:br/>')}</w:r>`

function mathExtension(block: boolean): TokenizerExtension {
  return {
    name: 'wordMath', level: block ? 'block' : 'inline',
    start: source => { const i = source.search(block ? /\$\$|\\\[/ : /\$|\\[([]/); return i < 0 ? undefined : i },
    tokenizer(source) {
      const open = block ? /^(\$\$|\\\[)/.exec(source)?.[0] : /^(\$\$|\$(?!\s)|\\\(|\\\[)/.exec(source)?.[0]
      if (!open) return
      const close = ({ '$': '$', '$$': '$$', '\\(': '\\)', '\\[': '\\]' } as Record<string, string>)[open]!
      let end = open.length
      while ((end = source.indexOf(close, end)) >= 0) {
        let escapes = 0
        for (let p = end - 1; p >= 0 && source[p] === '\\'; p--) escapes++
        if (escapes % 2 === 0) break
        end += close.length
      }
      if (end < 0) {
        if (open !== '$') throw new Error('公式缺少结束标记，请先修正后导出。')
        return // A lone dollar sign may be currency.
      }
      const body = source.slice(open.length, end)
      if (open === '$' && (!body.trim() || /\s$/.test(body) || /[`\r\n]/.test(body))) return
      if (block && !/^(?:[ \t]*\n|[ \t]*$)/.test(source.slice(end + close.length))) return
      return { type: 'wordMath', raw: source.slice(0, end + close.length), text: body.trim(), display: open !== '$' }
    },
  }
}

export function wordFilename(title = ''): string {
  const safe = title.replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ').replace(/\s+/g, ' ').trim().replace(/[. ]+$/, '').slice(0, 60)
  return `MindCreek-${safe || '回答'}.docx`
}

export async function createAnswerWord(answer: WordAnswer): Promise<WordExport> {
  if (!answer.content.trim()) throw new Error('回答为空，无法导出。')
  if (answer.content.length > 200_000) throw new Error('回答过长，单次最多导出 200000 字符。')
  const warnings = new Set<string>(), refs = answer.references || []
  const usedRefs: CitationKnowledgeRef[] = [], hyperlinks: string[] = [], listDefinitions: { id: number; ordered: boolean; start: number }[] = []
  let equations = 0, tables = 0, tokenCount = 0
  const lexer = new Marked({ gfm: true, breaks: true, extensions: [mathExtension(true), mathExtension(false)] })
  // Decode entities without inserting input into an active document or fetching resources.
  const decode = (s: string) => s.replace(/&(?:#\d+|#x[\da-f]+|amp|lt|gt|quot|apos|nbsp);/gi, entity => {
    const element = document.createElement('textarea'); element.innerHTML = entity; return element.value
  })
  const check = (depth: number) => { if (depth > 32 || ++tokenCount > 20000) throw new Error('回答结构过于复杂，无法导出。') }
  function math(token: MathToken) {
    if (++equations > 300) throw new Error('单次最多导出 300 个公式。')
    try { return latexToWord(token.text, token.display) }
    catch (error) { throw new Error(`第 ${equations} 个公式无法导出：${token.text.slice(0, 100)}\n${error instanceof Error ? error.message : '不支持的公式语法'}`) }
  }
  function citation(raw: string): string {
    const attrs = Object.fromEntries([...raw.matchAll(/([\w-]+)\s*=\s*["']([^"']*)["']/g)].map(m => [m[1], decode(m[2]!)]))
    const id = resolveCitationChunkId(attrs.chunk_id || attrs.id || '', { doc: attrs.doc, kbId: attrs.kb_id }, refs)
    const ref = refs.find(r => r.id === id) || refs.find(r => r.knowledge_title === attrs.doc || r.knowledge_filename === attrs.doc)
    if (!ref) { warnings.add('部分引用未匹配到来源，已保留引用标签。'); return run(`[来源：${attrs.doc || attrs.title || '未匹配'}]`) }
    let index = usedRefs.indexOf(ref)
    if (index < 0) { index = usedRefs.length; usedRefs.push(ref) }
    return run(`[${index + 1}]`, '<w:vertAlign w:val="superscript"/>')
  }
  function inline(tokens: Token[], props = '', depth = 0): string {
    return tokens.map(token => {
      check(depth)
      const t = token as Tokens.Generic
      switch (t.type) {
        case 'wordMath': return math(token as MathToken)
        case 'text': case 'escape': return t.tokens ? inline(t.tokens, props, depth + 1) : run(decode(t.text), props)
        case 'strong': return inline(t.tokens!, props + '<w:b/>', depth + 1)
        case 'em': return inline(t.tokens!, props + '<w:i/>', depth + 1)
        case 'del': return inline(t.tokens!, props + '<w:strike/>', depth + 1)
        case 'codespan': return run(t.text, props + '<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:shd w:fill="F3F4F6"/>')
        case 'br': return '<w:r><w:br/></w:r>'
        case 'link': {
          const label = inline(t.tokens!, props, depth + 1)
          if (!/^(https?:\/\/|mailto:)/i.test(t.href) || /[\u0000-\u0020]/.test(t.href)) { warnings.add('不安全或应用内部链接已导出为文字。'); return label }
          hyperlinks.push(t.href)
          return `<w:hyperlink r:id="link${hyperlinks.length}">${label}</w:hyperlink>`
        }
        case 'image': warnings.add('图片以说明文字保留，未嵌入图片。'); return run(`[图片：${decode(t.text || '图片')}]`, props)
        case 'html': {
          if (/^<(kb|web)\b/i.test(t.raw)) return citation(t.raw)
          if (/^<br\s*\/?\s*>$/i.test(t.raw)) return '<w:r><w:br/></w:r>'
          warnings.add('HTML 标记按原文保留；建议使用 Markdown 表格和格式。')
          return run(t.raw, props)
        }
        default: throw new Error(`暂不支持回答格式 ${t.type}`)
      }
    }).join('')
  }
  function htmlTable(raw: string, props: string, depth: number): string | undefined {
    if (!/^\s*<table\b/i.test(raw)) return
    const template = document.createElement('template')
    template.innerHTML = raw // Inert fragment: never attached to the page.
    const table = template.content.querySelector('table')
    if (!table || template.content.querySelectorAll('table').length !== 1 || template.content.children.length !== 1) throw new Error('暂不支持嵌套或混合 HTML 表格，请使用 Markdown 表格。')
    const rows = Array.from(table.rows)
    const cols = rows[0]?.cells.length || 0
    if (!cols || rows.some(r => r.cells.length !== cols || Array.from(r.cells).some(c => c.colSpan !== 1 || c.rowSpan !== 1))) throw new Error('暂不支持 HTML 表格的合并单元格，请拆分后导出。')
    const cells = rows.map(r => Array.from(r.cells).map(cell => {
      if (cell.querySelector('img,script,style,iframe,object')) throw new Error('HTML 表格含暂不支持的嵌入内容。')
      for (const br of cell.querySelectorAll('br')) br.replaceWith('\n')
      const value = cell.textContent || '', tokens: Token[] = []
      for (const part of lexer.lexer(value)) {
        if (part.type === 'space') continue
        if (part.type === 'wordMath') tokens.push(part)
        else if (part.type === 'paragraph' || part.type === 'text') tokens.push(...(part.tokens || [part]))
        else throw new Error('HTML 单元格内容过于复杂，请使用 Markdown 表格。')
      }
      return { text: value, tokens, header: false, align: null }
    }))
    warnings.add('HTML 表格已转换为标准 Word 表格，原 HTML 样式未保留。')
    return blocks([{ type: 'table', raw, header: cells[0]!, rows: cells.slice(1), align: Array(cols).fill(null) }], props, depth + 1)
  }
  function blocks(tokens: Token[], props = '', depth = 0): string {
    return tokens.map(token => {
      check(depth)
      const t = token as Tokens.Generic
      switch (t.type) {
        case 'space': return ''
        case 'wordMath': return para(`<m:oMathPara>${math(token as MathToken)}</m:oMathPara>`, props)
        case 'paragraph': case 'text': return para(t.tokens ? inline(t.tokens, '', depth + 1) : run(decode(t.text)), props)
        case 'heading': return para(inline(t.tokens!, '', depth + 1), `<w:pStyle w:val="Heading${t.depth}"/>${props}`)
        case 'blockquote': return blocks(t.tokens!, props + '<w:ind w:left="360"/><w:pBdr><w:left w:val="single" w:sz="12" w:space="8" w:color="D9D9D9"/></w:pBdr>', depth + 1)
        case 'code': return t.text.split('\n').map((line: string) => para(run(line, '<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="19"/>'), '<w:pStyle w:val="Code"/>' + props)).join('')
        case 'hr': return para('', '<w:pBdr><w:bottom w:val="single" w:sz="4" w:color="D9D9D9"/></w:pBdr>')
        case 'list': {
          const id = listDefinitions.length + 1
          listDefinitions.push({ id, ordered: t.ordered, start: Number(t.start) || 1 })
          return t.items.map((item: Tokens.ListItem) => {
            const num = `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="${id}"/></w:numPr><w:ind w:left="${360 * (depth + 1)}" w:hanging="240"/>`
            const items = item.tokens
            return items.map((part, index) => blocks([part], index === 0 ? props + num : props, depth + 1)).join('')
          }).join('')
        }
        case 'table': {
          tables++
          const table = token as Tokens.Table, cols = table.header.length
          if (cols > 16 || table.rows.length > 500) throw new Error('表格过大（最多 16 列、500 行）。')
          const widths = table.header.map((_, i) => Math.min(40, Math.max(6, ...[table.header, ...table.rows].map(r => Array.from(r[i]?.text || '').length))))
          const total = widths.reduce((a, b) => a + b, 0), sizes = widths.map(n => Math.floor(n / total * 9360))
          sizes[cols - 1]! += 9360 - sizes.reduce((a, b) => a + b, 0)
          const border = ['top', 'left', 'bottom', 'right', 'insideH', 'insideV'].map(s => `<w:${s} w:val="single" w:sz="4" w:color="D9D9D9"/>`).join('')
          return `<w:tbl><w:tblPr><w:tblW w:w="9360" w:type="dxa"/><w:tblBorders>${border}</w:tblBorders><w:tblLayout w:type="fixed"/><w:tblCellMar><w:top w:w="100" w:type="dxa"/><w:left w:w="120" w:type="dxa"/><w:bottom w:w="100" w:type="dxa"/><w:right w:w="120" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>${sizes.map(n => `<w:gridCol w:w="${n}"/>`).join('')}</w:tblGrid>${[table.header, ...table.rows].map((r, index) => `<w:tr>${index === 0 ? '<w:trPr><w:tblHeader/></w:trPr>' : ''}${r.map((cell, i) => `<w:tc><w:tcPr><w:tcW w:w="${sizes[i]}" w:type="dxa"/>${index === 0 ? '<w:shd w:fill="E7EEF5"/>' : ''}<w:vAlign w:val="center"/></w:tcPr>${para(inline(cell.tokens, index === 0 ? '<w:b/>' : '', depth + 1), `<w:spacing w:after="0"/><w:jc w:val="${table.align[i] || 'left'}"/>`)}</w:tc>`).join('')}</w:tr>`).join('')}</w:tbl>${para('')}`
        }
        case 'html': return htmlTable(t.raw, props, depth) ?? para(inline([token], '', depth + 1), props)
        default: throw new Error(`暂不支持回答格式 ${t.type}`)
      }
    }).join('')
  }
  const title = (answer.title || 'MindCreek 回答').replace(/\s+/g, ' ').trim().slice(0, 200)
  let body = para(run(title), '<w:pStyle w:val="Title"/>') + blocks(lexer.lexer(answer.content))
  // Native retrieval metadata is already present on the visible answer. No new requests.
  for (const ref of refs) if (!usedRefs.includes(ref)) usedRefs.push(ref)
  if (usedRefs.length) body += para(run('参考来源'), '<w:pStyle w:val="Heading1"/>') + usedRefs.map((ref, index) => para(run(`[${index + 1}] ${ref.knowledge_title || ref.knowledge_filename || '知识库文档'}${ref.chunk_index !== undefined ? ` · 分块 ${ref.chunk_index}` : ''}`))).join('')
  const zip = new JSZip()
  zip.file('[Content_Types].xml', xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>${['document', 'styles', 'numbering', 'settings'].map(name => `<Override PartName="/word/${name}.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.${name === 'document' ? 'document.main' : name}+xml"/>`).join('')}</Types>`)
  zip.file('_rels/.rels', xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="document" Type="${R}/officeDocument" Target="word/document.xml"/></Relationships>`)
  zip.file('word/document.xml', xmlHeader + `<w:document xmlns:w="${W}" xmlns:m="${M}" xmlns:r="${R}"><w:body>${body}<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720"/></w:sectPr></w:body></w:document>`)
  zip.file('word/_rels/document.xml.rels', xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${['styles', 'numbering', 'settings'].map(n => `<Relationship Id="${n}" Type="${R}/${n}" Target="${n}.xml"/>`).join('')}${hyperlinks.map((href, i) => `<Relationship Id="link${i + 1}" Type="${R}/hyperlink" Target="${xml(href)}" TargetMode="External"/>`).join('')}</Relationships>`)
  zip.file('word/styles.xml', xmlHeader + `<w:styles xmlns:w="${W}"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei"/><w:color w:val="000000"/><w:sz w:val="22"/><w:lang w:val="en-US" w:eastAsia="zh-CN"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="140" w:line="300" w:lineRule="auto"/><w:widowControl/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>${[['Title', 36], ...Array.from({ length: 6 }, (_, i) => ['Heading' + (i + 1), 30 - i * 2])].map(([name, size]) => `<w:style w:type="paragraph" w:styleId="${name}"><w:name w:val="${name}"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="220" w:after="140"/></w:pPr><w:rPr><w:b/><w:color w:val="000000"/><w:sz w:val="${size}"/></w:rPr></w:style>`).join('')}<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0"/><w:shd w:fill="F3F4F6"/></w:pPr></w:style></w:styles>`)
  zip.file('word/numbering.xml', xmlHeader + `<w:numbering xmlns:w="${W}">${listDefinitions.map(l => `<w:abstractNum w:abstractNumId="${l.id}"><w:multiLevelType w:val="singleLevel"/><w:lvl w:ilvl="0"><w:start w:val="${l.start}"/><w:numFmt w:val="${l.ordered ? 'decimal' : 'bullet'}"/><w:lvlText w:val="${l.ordered ? '%1.' : '•'}"/><w:lvlJc w:val="left"/></w:lvl></w:abstractNum>`).join('')}${listDefinitions.map(l => `<w:num w:numId="${l.id}"><w:abstractNumId w:val="${l.id}"/></w:num>`).join('')}</w:numbering>`)
  zip.file('word/settings.xml', xmlHeader + `<w:settings xmlns:w="${W}" xmlns:m="${M}"><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat><m:mathPr><m:mathFont m:val="Cambria Math"/></m:mathPr></w:settings>`)
  const bytes = await zip.generateAsync({ type: 'uint8array', compression: 'DEFLATE' })
  return { bytes, filename: wordFilename(title), warnings: [...warnings], equations, tables }
}

export async function downloadAnswerWord(answer: WordAnswer): Promise<WordExport> {
  const result = await createAnswerWord(answer)
  const blob = new Blob([new Uint8Array(result.bytes)], { type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' })
  const url = URL.createObjectURL(blob), a = document.createElement('a')
  a.href = url; a.download = result.filename; a.click()
  setTimeout(() => URL.revokeObjectURL(url), 30_000)
  return result
}
