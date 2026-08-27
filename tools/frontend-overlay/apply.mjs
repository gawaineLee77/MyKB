#!/usr/bin/env node

import {
  appendFileSync,
  cpSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const repositoryRoot = resolve(scriptDir, '../..')
const frontendRoot = resolve(process.argv[2] || '')
const brandingRoot = resolve(process.argv[3] || resolve(repositoryRoot, 'branding/mindcreek'))
const productRoot = resolve(scriptDir, 'product/mindcreek')
const marker = 'MindCreek Stage 1 product theme'

if (!process.argv[2]) {
  throw new Error('usage: node tools/frontend-overlay/apply.mjs <frontend-copy> [branding-directory]')
}

const brand = JSON.parse(readFileSync(resolve(brandingRoot, 'brand.json'), 'utf8'))

function pathFor(relativePath) {
  return resolve(frontendRoot, relativePath)
}

function read(relativePath) {
  const target = pathFor(relativePath)
  if (!existsSync(target)) throw new Error(`missing upstream anchor file: ${relativePath}`)
  return readFileSync(target, 'utf8')
}

function write(relativePath, content) {
  writeFileSync(pathFor(relativePath), content)
}

function replaceExact(relativePath, before, after, expectedCount = 1) {
  const source = read(relativePath)
  const actualCount = source.split(before).length - 1
  if (actualCount !== expectedCount) {
    throw new Error(
      `${relativePath}: expected ${expectedCount} occurrence(s) of an upstream anchor, found ${actualCount}`,
    )
  }
  write(relativePath, source.split(before).join(after))
}

function replaceRegex(relativePath, pattern, after, expectedCount = 1) {
  const source = read(relativePath)
  const matches = source.match(pattern) || []
  if (matches.length !== expectedCount) {
    throw new Error(
      `${relativePath}: expected ${expectedCount} regex anchor(s), found ${matches.length}`,
    )
  }
  write(relativePath, source.replace(pattern, after))
}

const themePath = 'src/assets/theme/theme.css'
if (read(themePath).includes(marker)) {
  throw new Error('MindCreek overlay has already been applied to this frontend copy')
}

replaceExact('index.html', '<title>WeKnora</title>', `<title>${brand.name}</title>`)
replaceExact(
  'index.html',
  'content="WeKnora是一款基于大语言模型的文档理解与语义检索框架，专为结构复杂、内容异构的文档场景而打造。"',
  `content="${brand.name} 是面向组织内部用户的私有知识库与智能体平台。"`,
)
replaceExact('index.html', './public/favicon.ico', '/mindcreek-favicon.png', 2)
replaceExact('embed.html', '<title>WeKnora Embed</title>', `<title>${brand.name} Embed</title>`)
replaceExact('embed.html', './public/favicon.ico', '/mindcreek-favicon.png')

replaceExact(
  'src/views/auth/Login.vue',
  `    <a href="https://github.com/Tencent/WeKnora" target="_blank" class="header-logo" :title="$t('common.github')">\n      <img src="@/assets/img/weknora.png" alt="WeKnora" class="logo-image" />\n    </a>`,
  `    <a href="/" class="header-logo mindcreek-brand" aria-label="${brand.name}">\n      <img src="@/assets/img/mindcreek-mark.png" alt="" class="logo-image mindcreek-logo-image" />\n      <span class="mindcreek-wordmark">${brand.name}</span>\n    </a>`,
)
replaceRegex(
  'src/views/auth/Login.vue',
  /      <a href="https:\/\/weknora\.weixin\.qq\.com"[\s\S]*?      <\/a>\n\n/g,
  '',
)
replaceExact(
  'src/views/auth/Login.vue',
  'href="https://github.com/Tencent/WeKnora"',
  `href="${brand.repositoryUrl}"`,
)
replaceExact(
  'src/components/menu.vue',
  '                <img class="logo" src="@/assets/img/weknora.png" alt="">',
  `                <img class="logo mindcreek-logo-image" src="@/assets/img/mindcreek-mark.png" alt="">\n                <span class="mindcreek-wordmark">${brand.name}</span>`,
)

replaceExact(
  'src/router/index.ts',
  `        {
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("../views/knowledge/KnowledgeBaseList.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },`,
  `        {
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("@/mindcreek/KnowledgeLibrary.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        // MindCreek Stage 2 product module. The source lives outside the upstream tree.
        {
          path: "mindcreek/create",
          name: "mindcreekCreateKnowledgeSpace",
          component: () => import("@/mindcreek/CreateKnowledgeSpace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "mindcreek/notes/:kbId",
          name: "mindcreekNotesWorkspace",
          component: () => import("@/mindcreek/NotesWorkspace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "mindcreek/rag/:kbId",
          name: "mindcreekRAGWorkspace",
          component: () => import("@/mindcreek/RAGWorkspace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },`,
)
replaceExact(
  'src/utils/request.ts',
  `export function put<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.put<T>(url, data, config) as unknown as Promise<T>;
}

export function del<T = any>(url: string, data?: any): Promise<T> {`,
  `export function put<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.put<T>(url, data, config) as unknown as Promise<T>;
}

export function patch<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.patch<T>(url, data, config) as unknown as Promise<T>;
}

export function del<T = any>(url: string, data?: any): Promise<T> {`,
)
const translations = {
  'src/i18n/locales/en-US.ts': [
    ['Welcome to WeKnora', `Welcome to ${brand.name}`],
    [
      'Everything starts here: upload documents, web pages or FAQs and WeKnora parses and indexes them automatically. Click here to open knowledge bases.',
      `Everything starts here: add documents, web pages or FAQs and ${brand.name} prepares them for reliable retrieval. Click here to open knowledge bases.`,
    ],
    ['New to WeKnora?', `New to ${brand.name}?`],
    [
      'RAG Q&A, ReAct Agent and Wiki — an LLM-powered enterprise knowledge framework',
      'Private notes, reliable RAG and governed knowledge sharing for your organization',
    ],
    ['Create your account and start using WeKnora', `Create your account and start using ${brand.name}`],
    ['Hi, I am WeKnora — your knowledge, within reach', `Hi, I am ${brand.name} — your knowledge, within reach`],
    [
      'LLM-Powered Enterprise Knowledge Framework',
      'Private knowledge that flows where work happens',
    ],
    [
      'RAG retrieval, agentic reasoning and Wiki knowledge bases — so your documents are truly understood and put to work',
      'Personal notes, reliable RAG and governed sharing in one self-hosted workspace',
    ],
  ],
  'src/i18n/locales/zh-CN.ts': [
    ['欢迎使用 WeKnora', `欢迎使用 ${brand.name}`],
    [
      '知识库是一切的起点：上传文档、网页或 FAQ，WeKnora 会自动解析并建立索引。点击这里进入知识库。',
      `知识库是一切的起点：添加文档、网页或 FAQ，${brand.name} 会为可靠检索完成处理。点击这里进入知识库。`,
    ],
    ['首次使用 WeKnora？', `首次使用 ${brand.name}？`],
    [
      'RAG 问答、ReAct 智能体与 Wiki 知识库，大模型驱动的企业级知识框架',
      '面向组织的私人笔记、可靠 RAG 与受控知识共享',
    ],
    ['创建账户并开始使用 WeKnora', `创建账户并开始使用 ${brand.name}`],
    ['Hi，我是 WeKnora，让你的知识触手可及', `Hi，我是 ${brand.name}，让你的知识触手可及`],
    ['大模型驱动的企业级知识框架', '让组织知识持续汇聚、可靠流动'],
    [
      'RAG 检索、智能体推理、Wiki 知识库，让文档真正被理解和运用',
      '在一个私有化工作空间中统一管理个人笔记、可靠 RAG 与受控共享',
    ],
  ],
}

for (const [relativePath, replacements] of Object.entries(translations)) {
  for (const [before, after] of replacements) replaceExact(relativePath, before, after)
}

mkdirSync(pathFor('src/assets/img'), { recursive: true })
mkdirSync(pathFor('public'), { recursive: true })
mkdirSync(pathFor('src/mindcreek'), { recursive: true })
cpSync(productRoot, pathFor('src/mindcreek'), { recursive: true })
copyFileSync(resolve(brandingRoot, 'assets/mindcreek-mark-ui.png'), pathFor('src/assets/img/mindcreek-mark.png'))
copyFileSync(resolve(brandingRoot, 'assets/mindcreek-favicon.png'), pathFor('public/mindcreek-favicon.png'))
appendFileSync(pathFor(themePath), `\n\n${readFileSync(resolve(brandingRoot, 'theme.css'), 'utf8')}\n`)

console.log(`Applied ${brand.name} branding and Stage 2 product modules to ${frontendRoot}`)
