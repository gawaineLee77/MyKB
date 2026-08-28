<template>
  <section class="catalog">
    <form class="filters" @submit.prevent="search"><input v-model.trim="query" :placeholder="text.searchHint" /><input v-model.trim="tag" :placeholder="text.tagHint" /><select v-model="accessMode"><option value="">{{ text.allModes }}</option><option value="subscriber">{{ text.subscriber }}</option><option value="organization_public">{{ text.public }}</option></select><button class="primary" type="submit">{{ text.search }}</button></form>
    <div v-if="loading && !page.items.length" class="state"><t-loading /> {{ text.loading }}</div>
    <div v-else-if="error" class="state error">{{ error }} <button type="button" @click="load">{{ text.retry }}</button></div>
    <div v-else-if="!page.items.length" class="state"><t-icon name="search" /><strong>{{ text.empty }}</strong><p>{{ text.emptyHint }}</p></div>
    <div v-else class="grid">
      <article v-for="item in page.items" :key="item.publication.id" @click="open(item)">
        <header><span>{{ item.publication.access_mode === 'organization_public' ? text.public : text.subscriber }}</span><b v-if="item.updated">{{ text.updated }}</b></header>
        <h2>{{ item.publication.title }}</h2><p>{{ item.publication.description || text.noDescription }}</p>
        <div class="tags"><small v-for="value in item.publication.tags" :key="value">#{{ value }}</small></div>
        <footer><span>{{ text.by }} {{ item.publication.publisher_id.slice(0, 12) }}</span><button v-if="item.subscribed" type="button" @click.stop="unsubscribe(item)">{{ text.unsubscribe }}</button><button v-else-if="item.can_subscribe" class="primary" type="button" @click.stop="subscribe(item)">{{ text.subscribe }}</button><span v-else-if="item.publication.publisher_id">{{ text.owned }}</span></footer>
      </article>
    </div>
    <footer v-if="page.total > page.page_size" class="pagination"><button type="button" :disabled="page.page <= 1" @click="change(-1)">‹</button><span>{{ page.page }} / {{ Math.ceil(page.total/page.page_size) }}</span><button type="button" :disabled="page.page*page.page_size >= page.total" @click="change(1)">›</button></footer>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { listCatalog, markPublicationSeen, subscribePublication, unsubscribePublication, type CatalogItem, type CatalogPage, type PublicationAccessMode } from './api'
const emit = defineEmits<{ changed: [] }>(), { locale } = useI18n(), router = useRouter()
const query=ref(''),tag=ref(''),accessMode=ref<PublicationAccessMode|''>(''),loading=ref(false),error=ref('')
const page=reactive<CatalogPage>({items:[],total:0,page:1,page_size:24})
const words={en:{searchHint:'Search titles, descriptions, guidance…',tagHint:'Tag',allModes:'All access modes',subscriber:'Subscribers only',public:'Organization public',search:'Search',loading:'Loading catalog…',retry:'Retry',empty:'No publications match',emptyHint:'Try a broader search or another tag.',updated:'Updated',noDescription:'No description provided.',by:'Published by',subscribe:'Subscribe',unsubscribe:'Unsubscribe',owned:'Your publication'},zh:{searchHint:'搜索标题、描述和使用说明…',tagHint:'标签',allModes:'全部访问模式',subscriber:'仅订阅者',public:'组织内公开',search:'搜索',loading:'正在加载目录…',retry:'重试',empty:'没有匹配的发布内容',emptyHint:'尝试更宽泛的搜索或其他标签。',updated:'有更新',noDescription:'暂无描述。',by:'发布者',subscribe:'订阅',unsubscribe:'取消订阅',owned:'你的发布'}}
const text=computed(()=>locale.value.startsWith('zh')?words.zh:words.en)
function messageOf(value:unknown){const body=value&&typeof value==='object'&&'response'in value?(value as any).response?.data:undefined;return body?.error?.message||String((value as any)?.message||value)}
async function load(){loading.value=true;error.value='';try{Object.assign(page,await listCatalog({query:query.value,tag:tag.value,accessMode:accessMode.value,page:page.page}))}catch(value){error.value=messageOf(value);page.items=[]}finally{loading.value=false}}
async function search(){page.page=1;await load()} async function change(delta:number){page.page+=delta;await load()}
async function subscribe(item:CatalogItem){try{await subscribePublication(item.publication.id);emit('changed');await load()}catch(value){error.value=messageOf(value)}}
async function unsubscribe(item:CatalogItem){try{await unsubscribePublication(item.publication.id);emit('changed');await load()}catch(value){error.value=messageOf(value)}}
async function open(item:CatalogItem){if(!item.can_read)return;if(item.subscribed&&item.updated)await markPublicationSeen(item.publication.id);void router.push(`/platform/mindcreek/rag/${item.publication.knowledge_base_id}`)}
onMounted(load)
</script>

<style scoped>
.catalog{max-width:1180px;margin:0 auto}.filters{display:grid;grid-template-columns:2fr 1fr 1fr auto;gap:9px;margin-bottom:18px}.filters input,.filters select,.filters button{padding:10px;border:1px solid #cfddd7;border-radius:9px;background:#fff}.primary{color:#fff!important;border-color:#207d61!important;background:#207d61!important}.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:15px}.grid article{min-height:220px;padding:20px;display:flex;flex-direction:column;border:1px solid #dce8e3;border-radius:16px;background:#fff;box-shadow:0 11px 32px rgba(43,84,72,.055);cursor:pointer}.grid header,.grid footer{display:flex;justify-content:space-between;gap:9px;align-items:center}.grid header span{color:#257a60;font-size:10px;font-weight:750;text-transform:uppercase}.grid header b{padding:4px 7px;border-radius:99px;color:#875f12;background:#fff0c9;font-size:10px}.grid h2{margin:16px 0 7px}.grid p{flex:1;margin:0;color:#74887f;line-height:1.5}.tags{min-height:28px;padding:10px 0;display:flex;flex-wrap:wrap;gap:5px}.tags small{color:#35715e}.grid footer{padding-top:13px;border-top:1px solid #e8efec;color:#778a83;font-size:11px}.grid footer button{padding:7px 9px;border:1px solid #cbdad4;border-radius:8px;background:#fff}.state{min-height:330px;display:flex;flex-direction:column;gap:9px;align-items:center;justify-content:center;color:#72877f;text-align:center}.state.error{color:#9b4040}.pagination{padding:22px;display:flex;justify-content:center;gap:12px;align-items:center}.pagination button{padding:7px 12px;border:1px solid #cbdad4;border-radius:8px;background:#fff}@media(max-width:700px){.filters{grid-template-columns:1fr}.grid{grid-template-columns:1fr}}
</style>
