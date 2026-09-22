// Real native UI, nginx and synthetic browser OAuth. No response interception.
import {createRequire} from 'node:module'
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs'
import {createHash} from 'node:crypto'
const {chromium}=createRequire(import.meta.url)('playwright')
const root=process.env.R5_ROOT,origin='http://mindcreek.localhost:18685',portal='http://localhost:18687'
const f=JSON.parse(readFileSync(process.env.R5_BROWSER_SECRETS,'utf8'))
const out=`${root}/docs/assets/r5`;mkdirSync(out,{recursive:true})
const build=JSON.parse(readFileSync(`${root}/docs/plans/evidence/r5-ui-build.json`,'utf8'))
const report={scope:'Real native API + nginx + Vue UI + loopback synthetic OAuth provider. Independent localhost portal and mindcreek.localhost product origins; no production IdP/models.',...build,checks:[],screenshots:[],requests:[],probe_sha256:createHash('sha256').update(readFileSync(import.meta.filename)).digest('hex')}
report.status='running'
const browser=await chromium.launch({executablePath:process.env.R5_CHROME_EXECUTABLE || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true,args:['--test-third-party-cookie-phaseout']})
// Chromium resolves *.localhost natively; the Node diagnostics use the same
// loopback listener without requiring OS-wide host-file changes.
const localFetch=(url,options)=>fetch(url.replace(origin,'http://127.0.0.1:18685'),options)
const check=(name,condition=true)=>{if(!condition)throw new Error(name);report.checks.push({name,passed:true});console.log('PASS '+name)}
const screenshot=async(page,name)=>{await page.screenshot({path:`${out}/${name}.png`,fullPage:true,animations:'disabled'});report.screenshots.push(`docs/assets/r5/${name}.png`)}
const prefix=`/api/v1/mindcreek/assistant/${f.tenant}/${f.channel}`
async function adminCall(path,method,body){const r=await localFetch(origin+path,{method,headers:{Authorization:'Bearer '+f.admin,'X-Tenant-ID':String(f.tenant),'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined});if(!r.ok)throw new Error('Admin fixture operation '+r.status);return r.json()}
async function context(){const c=await browser.newContext({viewport:{width:1440,height:1000},locale:'zh-CN'});await c.route('**/*',route=>{const url=new URL(route.request().url());return ['127.0.0.1','localhost','mindcreek.localhost'].includes(url.hostname)||['data:','blob:'].includes(url.protocol)?route.continue():route.abort()});return c}
async function login(name){
  const ctx=await context()
  // Popup login legitimately persists the existing first-party R2 session.
  // Browsers may share that origin's storage with the iframe. Prove the iframe
  // neither writes credentials nor depends on reading that shared storage.
  await ctx.addInitScript(()=>{
    if(!/^\/assistant\/\d+\//.test(location.pathname))return
    window.assistantCredentialWrites=[]
    const get=Storage.prototype.getItem,set=Storage.prototype.setItem
    const credential=key=>['weknora_token','weknora_refresh_token'].includes(key)
    Storage.prototype.getItem=function(key){return credential(key)?null:get.call(this,key)}
    Storage.prototype.setItem=function(key,value){if(credential(key)){window.assistantCredentialWrites.push(key);throw new Error('Iframe credential persistence forbidden')}return set.call(this,key,value)}
  })
  const page=await ctx.newPage();const errors=[]
  page.on('pageerror',e=>errors.push(e.message))
  page.on('response',r=>{const u=new URL(r.url());if(u.pathname.startsWith('/api/'))report.requests.push({path:u.pathname,status:r.status(),method:r.request().method()})})
  await page.goto(portal)
  await page.evaluate(()=>{window.hostMessages=[];window.addEventListener('message',event=>window.hostMessages.push(event.data))})
  await page.mouse.click(1340,954)
  await page.waitForFunction(()=>document.querySelector('div')!==null)
  let frame
  for(let n=0;n<100;n++){frame=page.frames().find(x=>x.url().includes('/assistant/'));if(frame)break;await new Promise(r=>setTimeout(r,100))}
  if(!frame)throw new Error('Assistant iframe missing')
  await frame.getByRole('button',{name:'企业登录',exact:true}).waitFor()
  const popupEvent=page.waitForEvent('popup')
  await frame.getByRole('button',{name:'企业登录',exact:true}).click()
  const popup=await popupEvent
  await popup.waitForURL('http://localhost:18686/**')
  await popup.locator('input[name=employee]').fill('r5-browser-'+name)
  if(name==='alice')await screenshot(popup,'after-synthetic-login-popup')
  await popup.getByRole('button',{name:'登录',exact:true}).click()
  await frame.locator('textarea').waitFor({timeout:45000})
  check(`popup.${name}.synthetic_oauth_real_callback`)
  check(`popup.${name}.credential_not_posted_to_host`,await page.evaluate(()=>window.hostMessages.every(x=>!x.token&&!x.refresh_token)))
  check(`popup.${name}.iframe_did_not_persist_token`,await frame.evaluate(()=>window.assistantCredentialWrites.length===0&&!localStorage.getItem('weknora_token')))
  return {ctx,page,frame,errors}
}
try {
  // Before = previously shipped R4 bundle, separate cached image and same synthetic API.
  const manager=await context();await manager.addInitScript(({token,tenant})=>{localStorage.setItem('weknora_token',token);localStorage.setItem('weknora_selected_tenant_id',String(tenant));localStorage.setItem('mindcreek_auth_kind','corporate');localStorage.setItem('locale','zh-CN');localStorage.setItem('weknora:new-user-guide-done:v1','1');localStorage.setItem('weknora:contextual-guide-agent-list:v1','1')},{token:f.manager,tenant:f.tenant})
  const page=await manager.newPage()
  await page.goto('http://127.0.0.1:18688/platform/agents')
  await page.getByRole('heading',{name:'智能体',exact:true}).waitFor()
  await page.getByText('企业知识小助手 · 合成验证',{exact:true}).first().waitFor()
  check('before.r4_has_no_employee_publish_button',await page.getByRole('button',{name:'发布到网页',exact:true}).count()===0)
  await screenshot(page,'before-agent-list')
  await page.goto(origin+'/platform/agents')
  await page.getByRole('button',{name:'发布到网页',exact:true}).click()
  await page.getByRole('heading',{name:'发布智能体',exact:true}).waitFor()
  await page.locator('#assistant-agent').selectOption(f.agent)
  await page.getByRole('button',{name:/员工门户助手/}).click()
  await page.getByRole('button',{name:'保存设置',exact:true}).click()
  await page.getByText('已保存',{exact:true}).waitFor()
  await screenshot(page,'after-channel-publishing')
  await page.locator('pre').scrollIntoViewIfNeeded();await screenshot(page,'after-embed-code')
  check('manager.native_channels_edit_and_secret_free_code',!(await page.locator('pre').innerText()).includes('token'))
  await manager.close()
  const headers=await localFetch(`${origin}/assistant/${f.tenant}/${f.channel}?host_origin=${encodeURIComponent(portal)}`)
  check('nginx.dynamic_csp_and_no_xframe_conflict',headers.ok&&headers.headers.get('content-security-policy')?.includes(portal)&&!headers.headers.has('x-frame-options'))
  check('nginx.main_site_retains_frame_protection',(await localFetch(origin+'/')).headers.get('x-frame-options')==='SAMEORIGIN')
  check('nginx.malformed_frame_origin_denied',(await localFetch(`${origin}/assistant/${f.tenant}/${f.channel}?host_origin=*`)).status===403)
  const alice=await login('alice')
  await screenshot(alice.page,'after-employee-login')
  await alice.frame.locator('input[type=file]').nth(0).setInputFiles(f.image)
  await alice.frame.locator('input[type=file]').nth(1).setInputFiles({name:'synthetic.txt',mimeType:'text/plain',buffer:Buffer.from('Synthetic R5 attachment.')})
  await alice.frame.locator('textarea').fill('What is the synthetic recovery code?')
  await alice.frame.locator('textarea').press('Enter')
  await alice.frame.locator('.messages article').nth(1).waitFor({timeout:45000})
  await alice.frame.locator('.embed-bot-msg').getByText('MindCreek synthetic answer grounded in the retrieved knowledge.',{exact:true}).waitFor({timeout:45000})
  await screenshot(alice.page,'after-employee-answer')
  const session=await alice.frame.locator('select').inputValue()
  check('employee.sse_answer',Boolean(session))
  await alice.frame.getByRole('button',{name:'新建对话',exact:true}).click()
  await alice.frame.locator('select').selectOption(session)
  await alice.frame.locator('.messages article').nth(1).waitFor()
  check('employee.history_reload')
  await alice.frame.waitForFunction(()=>Array.from(document.querySelectorAll('.embed-user-msg img')).some(img=>img.src.startsWith('blob:')&&img.naturalWidth>0))
  check('employee.persisted_image_reauthorized_and_displayed')
  await screenshot(alice.page,'after-history-image')
  await alice.frame.locator('.tree-root-expand').click()
  await alice.frame.locator('.has-reference-trigger').first().click()
  await alice.frame.locator('.chat-references-panel').waitFor()
  await alice.frame.waitForFunction(()=>{const panel=document.querySelector('.chat-references-panel');return panel&&!panel.classList.contains('references-panel-enter-active')&&getComputedStyle(panel).transform==='none'})
  check('employee.native_reference_drawer',await alice.frame.locator('.chat-references-panel').getByText('R5 操作指南',{exact:true}).count()===1)
  await screenshot(alice.page,'after-knowledge-references')
  await alice.frame.locator('.chat-references-panel__close').click()
  const bob=await login('bob')
  check('employee.second_user_history_isolated',!(await bob.frame.locator('select').innerHTML()).includes(session))
  await adminCall(`/api/v1/mindcreek/assistant/channels/${f.channel}`,'PUT',{...f.settings,enabled:false})
  await alice.frame.locator('textarea').fill('Try after revocation')
  await alice.frame.locator('textarea').press('Enter')
  await alice.frame.getByRole('button',{name:'企业登录',exact:true}).waitFor()
  check('employee.disabled_channel_invalidates_next_request')
  await screenshot(alice.page,'after-channel-revoked')
  check('browser.no_unhandled_errors',!alice.errors.length&&!bob.errors.length)
  await adminCall(`/api/v1/mindcreek/assistant/channels/${f.channel}`,'PUT',f.settings)
  await alice.ctx.close();await bob.ctx.close();report.status='passed'
}catch(e){report.status='failed';report.error=String(e);for(const c of browser.contexts())for(const page of c.pages()){console.error('PAGE',page.url(),(await page.locator('body').innerText().catch(()=>'' )).slice(0,2400));await screenshot(page,'failure-'+report.screenshots.length)}throw e}
finally{await browser.close();writeFileSync(`${root}/docs/plans/evidence/r5-browser.json`,JSON.stringify(report,null,2)+'\n')}
