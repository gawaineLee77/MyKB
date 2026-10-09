// Additive product-owned adapter; never write into the upstream checkout.
export function applyWordExport({ read, write, replaceExact: exact }) {
  const bot = 'src/views/chat/components/botmsg.vue'
  const agent = 'src/views/chat/components/AgentStreamDisplay.vue'
  for (const path of [bot, agent]) {
    const anchor = "import { copyWithToast } from '@/utils/clipboard';"
    exact(path, anchor, anchor + "\nimport WordExportButton from '@/mindcreek/WordExportButton.vue';")
  }
  exact(bot, '                <!-- Skill artifact download:', '                <WordExportButton v-if="!embeddedMode" :content="getActualContent()" :title="userQuery" :references="session.knowledge_references" />\n                <!-- Skill artifact download:')
  exact(agent, '                <!-- Skill artifact download:', '                <WordExportButton :content="getActualContent(event)" :title="userQuery" :references="session.knowledge_references" />\n                <!-- Skill artifact download:')
  // JSZip is already locked by native docx-preview; declare our direct use too.
  const pkg = JSON.parse(read('package.json')), lock = JSON.parse(read('package-lock.json'))
  if (lock.packages['node_modules/jszip']?.version !== '3.10.1') throw new Error('Review the Word export ZIP dependency on upstream upgrades')
  pkg.dependencies.jszip = '3.10.1'
  lock.packages[''].dependencies.jszip = '3.10.1'
  write('package.json', JSON.stringify(pkg, null, 2) + '\n')
  write('package-lock.json', JSON.stringify(lock, null, 2) + '\n')
}
