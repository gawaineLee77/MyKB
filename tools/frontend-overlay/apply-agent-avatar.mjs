// Product-owned image upload and rendering for the pinned native Agent UI.
export function applyAgentAvatarUI({ read, write, replaceExact: exact }) {
  const avatar = 'src/components/AgentAvatar.vue'
  exact(avatar, "import { computed } from 'vue';", "import { computed } from 'vue';\nimport AgentIconPicture from '@/mindcreek/AgentIconPicture.vue';\nimport { agentIconLabel } from '@/mindcreek/agent-icon-policy';")
  exact(avatar, '  name: string;', '  name: string;\n  avatar?: string;\n  agentId?: string;')
  exact(avatar, '<span class="agent-avatar-letter" :style="letterStyle">{{ letter }}</span>', '<span class="agent-avatar-letter" :style="letterStyle">{{ agentIconLabel(avatar) || letter }}</span>\n    <AgentIconPicture :avatar="avatar" :agent-id="agentId" :name="name" />')

  const editor = 'src/views/agent/AgentEditorModal.vue'
  exact(editor, '<script setup lang="ts">', '<script setup lang="ts">\nimport AgentIconEditor from \'@/mindcreek/AgentIconEditor.vue\';')
  exact(editor, "const defaultFormData = {\n  name: '',", "const avatarUploadBusy = ref(false);\nconst avatarDraftReference = ref('');\nconst defaultFormData = {\n  avatar: '',\n  name: '',")
  exact(editor, '<AgentAvatar v-else :name="formData.name || \'?\'" size="medium" />', '<AgentAvatar v-else :name="formData.name || \'?\'" :avatar="formData.avatar" :agent-id="formData.avatar === avatarDraftReference ? undefined : formData.id" size="medium" />')
  exact(editor, '                    <!-- 描述 -->', `                    <div v-if="!isBuiltinAgent" class="setting-row">
                      <div class="setting-info"><label>智能体图标</label><p class="desc">上传图片，让列表和对话入口更容易辨认。</p></div>
                      <div class="setting-control"><AgentIconEditor v-model="formData.avatar" :name="formData.name" :agent-id="formData.id" @update:model-value="avatarDraftReference = $event" @busy="avatarUploadBusy = $event" /></div>
                    </div>

                    <!-- 描述 -->`)
  exact(editor, 'const handleSave = async () => {', "const handleSave = async () => {\n  if (avatarUploadBusy.value) { MessagePlugin.warning('请等待图标上传完成。'); return; }")

  const list = 'src/views/agent/AgentList.vue'
  exact(list, '<div v-else-if="agent.avatar" class="builtin-avatar agent-emoji">{{ agent.avatar }}</div>\n                  <AgentAvatar v-else :name="agent.name" size="small" />', '<AgentAvatar v-else :name="agent.name" :avatar="agent.avatar" :agent-id="agent.id" size="small" />', 2)
  exact(list, '<div v-if="shared.agent?.avatar" class="builtin-avatar agent-emoji">{{ shared.agent.avatar }}</div>\n                  <AgentAvatar v-else :name="shared.agent?.name" size="small" />', '<AgentAvatar :name="shared.agent?.name" :avatar="shared.agent?.avatar" :agent-id="shared.agent?.id" size="small" />')

  const selector = 'src/components/AgentSelector.vue'
  exact(selector, '<div v-else-if="agent.avatar" class="builtin-avatar">{{ agent.avatar }}</div>', '<AgentAvatar v-else-if="agent.avatar" :name="agent.name" :avatar="agent.avatar" :agent-id="agent.id" size="small" />')
  exact(selector, '<AgentAvatar :name="agent.name" size="small" />', '<AgentAvatar :name="agent.name" :avatar="agent.avatar" :agent-id="agent.id" size="small" />')
  exact(selector, '<AgentAvatar :name="shared.agent.name" size="small" />', '<AgentAvatar :name="shared.agent.name" :avatar="shared.agent.avatar" :agent-id="shared.agent.id" size="small" />')
  exact(selector, '<div v-else-if="activeDetail.agent.avatar" class="builtin-avatar detail-icon">{{ activeDetail.agent.avatar\n              }}</div>\n            <AgentAvatar v-else :name="activeDetail.agent.name" size="small" />', '<AgentAvatar v-else :name="activeDetail.agent.name" :avatar="activeDetail.agent.avatar" :agent-id="activeDetail.agent.id" size="small" />')

  const input = 'src/components/Input-field.vue'
  exact(input, '<script setup lang="ts">', '<script setup lang="ts">\nimport AgentAvatar from \'@/components/AgentAvatar.vue\';')
  exact(input, '<span class="agent-mode-text">', '<AgentAvatar v-if="isCustomAgent" :name="selectedAgent.name" :avatar="selectedAgent.avatar" :agent-id="selectedAgent.id" size="small" />\n            <span class="agent-mode-text">')
}
