import katex from 'katex'

export const xml = (value: string) => value.replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, '')
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;')
const children = (e: Element) => Array.from(e.children)
const text = (e: Element) => e.textContent || ''
const mathRun = (s: string, props = '') => `<m:r>${props ? `<m:rPr>${props}</m:rPr>` : ''}<w:rPr><w:rFonts w:ascii="Cambria Math" w:hAnsi="Cambria Math"/></w:rPr><m:t xml:space="preserve">${xml(s)}</m:t></m:r>`
const slot = (name: string, content: string) => `<m:${name}>${content}</m:${name}>`
const fail = (detail: string): never => { throw new Error(detail) }
const operators = new Set(Array.from('∑∏∐∫∬∭∮∯∰⋂⋃⋀⋁'))
const labelledArrows = new Set(['←', '→', '↔'])

function labelledArrow(e: Element): string | undefined {
  const c = children(e), base = c[0], tag = e.localName
  if (!base || base.localName !== 'mo' || base.getAttribute('stretchy') !== 'true' || !labelledArrows.has(text(base))) return
  if (c.length !== (tag === 'munderover' ? 3 : 2) || c.slice(1).some(label => label.localName !== 'mpadded')) return
  const labels = c.slice(1).map(label => {
    // Only KaTeX xArrow's symmetric horizontal label padding is supported.
    // Other mpadded nodes may move, overlap or hide content and must still fail.
    if (label.attributes.length !== 2 || label.getAttribute('width') !== '+0.6em' || label.getAttribute('lspace') !== '0.3em') fail('暂不支持此箭头标注的排版')
    return row(children(label))
  })
  const above = tag === 'munder' ? '' : labels[tag === 'munderover' ? 1 : 0]!
  const below = tag === 'mover' ? '' : labels[0]!
  if (!above && !below) return mathRun(text(base))
  // A grouping character stretches to its editable label; plain limits over
  // a text arrow would leave a short glyph underneath a long annotation.
  const pos = above ? 'bot' : 'top'
  const group = `<m:groupChr><m:groupChrPr><m:chr m:val="${xml(text(base))}"/><m:pos m:val="${pos}"/><m:vertJc m:val="${pos}"/></m:groupChrPr>${slot('e', '<m:argPr><m:argSz m:val="-1"/></m:argPr>' + (above || below))}</m:groupChr>`
  return above && below ? `<m:limLow>${slot('e', group)}${slot('lim', below)}</m:limLow>` : group
}

function nary(e: Element): { char: string; sub: string; sup: string; loc: string } | undefined {
  if (e.localName === 'mo' && operators.has(text(e))) return { char: text(e), sub: '', sup: '', loc: 'subSup' }
  const c = children(e), base = c[0]
  if (!base || base.localName !== 'mo' || !operators.has(text(base))) return
  if (!['msub', 'msup', 'msubsup', 'munder', 'mover', 'munderover'].includes(e.localName)) return
  const upperOnly = ['msup', 'mover'].includes(e.localName)
  return { char: text(base), sub: upperOnly ? '' : convert(c[1]!), sup: upperOnly ? convert(c[1]!) : c[2] ? convert(c[2]) : '', loc: e.localName.startsWith('ms') ? 'subSup' : 'undOvr' }
}

function row(items: Element[]): string {
  // KaTeX expresses stretchy fences as operators around a row/matrix.
  const first = items[0], last = items.at(-1)
  if (first?.getAttribute('fence') === 'true' && first.getAttribute('stretchy') !== 'false') {
    const close = last !== first && last?.getAttribute('fence') === 'true' && last.getAttribute('stretchy') !== 'false'
    return `<m:d><m:dPr><m:begChr m:val="${xml(text(first))}"/><m:endChr m:val="${close ? xml(text(last!)) : ''}"/></m:dPr>${slot('e', row(items.slice(1, close ? -1 : undefined)))}</m:d>`
  }
  let result = ''
  for (let i = 0; i < items.length; i++) {
    const op = nary(items[i]!)
    if (!op) { result += convert(items[i]!); continue }
    const next = items[i + 1]
    const operand = next && !(next.localName === 'mo' && /^[=+−,;]$/.test(text(next))) ? convert(items[++i]!) : ''
    result += `<m:nary><m:naryPr><m:chr m:val="${xml(op.char)}"/><m:limLoc m:val="${op.loc}"/><m:subHide m:val="${op.sub ? 0 : 1}"/><m:supHide m:val="${op.sup ? 0 : 1}"/></m:naryPr>${slot('sub', op.sub)}${slot('sup', op.sup)}${slot('e', operand)}</m:nary>`
  }
  return result
}

function convert(e: Element): string {
  const c = children(e), tag = e.localName
  const at = (i: number) => c[i] ? convert(c[i]!) : fail(`缺少 ${tag} 的公式项`)
  switch (tag) {
    case 'math': case 'mrow': case 'mtd': return row(c)
    case 'semantics': return at(0)
    case 'annotation': return ''
    case 'mstyle':
      if (Array.from(e.attributes).some(a => !['displaystyle', 'scriptlevel'].includes(a.name))) fail('暂不支持此公式样式')
      return row(c)
    case 'mi': case 'mn': case 'mo': case 'mtext': {
      const variant = e.getAttribute('mathvariant') || (tag === 'mi' && Array.from(text(e)).length === 1 ? 'italic' : 'normal')
      const variants: Record<string, string> = { normal: '<m:sty m:val="p"/>', italic: '<m:sty m:val="i"/>', bold: '<m:sty m:val="b"/>', 'bold-italic': '<m:sty m:val="bi"/>', 'double-struck': '<m:scr m:val="double-struck"/>', script: '<m:scr m:val="script"/>', fraktur: '<m:scr m:val="fraktur"/>', 'sans-serif': '<m:scr m:val="sans-serif"/>', monospace: '<m:scr m:val="monospace"/>' }
      if (!(variant in variants)) fail(`暂不支持公式字体 ${variant}`)
      return mathRun(text(e), tag === 'mtext' ? '<m:nor/>' : variants[variant])
    }
    case 'mspace': return mathRun(' ')
    case 'mfrac': {
      const thickness = e.getAttribute('linethickness')
      if (thickness && !/^0(?:px|em|pt)?$/.test(thickness)) fail('暂不支持自定义分数线')
      return `<m:f>${thickness ? '<m:fPr><m:type m:val="noBar"/></m:fPr>' : ''}${slot('num', at(0))}${slot('den', at(1))}</m:f>`
    }
    case 'msqrt': return `<m:rad><m:radPr><m:degHide m:val="1"/></m:radPr><m:deg/>${slot('e', row(c))}</m:rad>`
    case 'mroot': return `<m:rad>${slot('deg', at(1))}${slot('e', at(0))}</m:rad>`
    case 'msub': return `<m:sSub>${slot('e', at(0))}${slot('sub', at(1))}</m:sSub>`
    case 'msup': return `<m:sSup>${slot('e', at(0))}${slot('sup', at(1))}</m:sSup>`
    case 'msubsup': return `<m:sSubSup>${slot('e', at(0))}${slot('sub', at(1))}${slot('sup', at(2))}</m:sSubSup>`
    case 'mover': case 'munder': case 'munderover': {
      const arrow = labelledArrow(e)
      if (arrow !== undefined) return arrow
      if (e.getAttribute('accent') === 'true' || e.getAttribute('accentunder') === 'true') {
        const accent = text(c[1]!)
        if (['¯', 'ˉ', '‾', '_', '―'].includes(accent)) return `<m:bar><m:barPr><m:pos m:val="${tag === 'munder' ? 'bot' : 'top'}"/></m:barPr>${slot('e', at(0))}</m:bar>`
        if (tag !== 'mover' || Array.from(accent).length !== 1) fail('暂不支持此上下方装饰符')
        return `<m:acc><m:accPr><m:chr m:val="${xml(accent)}"/></m:accPr>${slot('e', at(0))}</m:acc>`
      }
      let base = at(0)
      if (tag !== 'mover') base = `<m:limLow>${slot('e', base)}${slot('lim', at(1))}</m:limLow>`
      if (tag !== 'munder') base = `<m:limUpp>${slot('e', base)}${slot('lim', at(tag === 'munderover' ? 2 : 1))}</m:limUpp>`
      return base
    }
    case 'mtable': {
      if (e.hasAttribute('rowlines') || e.hasAttribute('columnlines')) fail('暂不支持带分隔线的公式矩阵')
      const width = Math.max(1, ...c.map(r => r.children.length))
      if (width > 16 || c.length > 64) fail('公式矩阵过大')
      if (c.some(r => r.localName !== 'mtr' || children(r).some(cell => cell.localName !== 'mtd'))) fail('暂不支持此公式表格')
      const align = (e.getAttribute('columnalign') || 'center').split(/\s+/)
      const columns = Array.from({ length: width }, (_, i) => `<m:mc><m:mcPr><m:count m:val="1"/><m:mcJc m:val="${['left', 'right', 'center'].includes(align[i] || align[0]!) ? align[i] || align[0] : 'center'}"/></m:mcPr></m:mc>`).join('')
      return `<m:m><m:mPr><m:mcs>${columns}</m:mcs></m:mPr>${c.map(r => `<m:mr>${Array.from({ length: width }, (_, i) => slot('e', r.children[i] ? convert(r.children[i]!) : '')).join('')}</m:mr>`).join('')}</m:m>`
    }
    default: return fail(`暂不支持公式结构 ${tag}`)
  }
}

/** Deliberately bounded subset: unsupported structure fails the entire export. */
export function latexToWord(latex: string, display = false): string {
  if (latex.length > 8000) fail('单个公式过长（最多 8000 字符）')
  // These commands have visual behavior absent from KaTeX's MathML output.
  if (/\\(?:tag|label|ref|eqref|html\w*|includegraphics|href|url|phantom|hphantom|vphantom|smash|raisebox|llap|rlap|clap|reflectbox)\b/.test(latex)) fail('公式含暂不支持的排版或外部资源命令')
  let denied = false
  const html = katex.renderToString(latex, { output: 'mathml', displayMode: display, throwOnError: true, strict: 'error', maxExpand: 1000, maxSize: 20, trust: () => { denied = true; return false } })
  if (denied) fail('公式含外部资源命令')
  const source = html.match(/<math\b[\s\S]*<\/math>/)?.[0] || fail('公式解析失败')
  const document = new DOMParser().parseFromString(source, 'application/xml')
  if (document.querySelector('parsererror')) fail('公式结构无效')
  if (document.querySelectorAll('*').length > 4000) fail('公式结构过于复杂')
  return `<m:oMath>${convert(document.documentElement)}</m:oMath>`
}
