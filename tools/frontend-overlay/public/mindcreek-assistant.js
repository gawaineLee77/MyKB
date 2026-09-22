// Public loader: only channel coordinates cross the host boundary. No tokens.
(() => {
  const script = document.currentScript;
  if (!script) return;
  const origin = new URL(script.src).origin;
  const space = script.dataset.space || '', channel = script.dataset.channel || '';
  if (!/^\d+$/.test(space) || !/^[\w-]{1,128}$/.test(channel)) return;
  const container = document.createElement('div');
  const root = container.attachShadow({mode:'closed'});
  const style = document.createElement('style');
  style.textContent = ':host{all:initial}button{position:fixed;right:24px;bottom:24px;border:0;border-radius:28px;background:#147c66;color:white;padding:15px 23px;font:600 14px system-ui;cursor:pointer;box-shadow:0 6px 24px #123c3933;z-index:2147483000}iframe{position:fixed;right:24px;bottom:88px;width:min(460px,calc(100vw - 32px));height:min(720px,calc(100dvh - 112px));border:1px solid #d6e7df;border-radius:16px;box-shadow:0 18px 72px #123c3933;z-index:2147483000;background:white}iframe[hidden]{display:none}';
  const button = document.createElement('button');button.type='button';button.textContent='知识小助手';button.setAttribute('aria-expanded','false');
  const frame = document.createElement('iframe');frame.title='MindCreek 企业知识小助手';frame.hidden=true;frame.referrerPolicy='no-referrer';
  // A top-level popup performs enterprise login; the iframe does not need cookies.
  frame.src = `${origin}/assistant/${space}/${channel}?host_origin=${encodeURIComponent(location.origin)}`;
  button.addEventListener('click',()=>{frame.hidden=!frame.hidden;button.textContent=frame.hidden?'知识小助手':'收起小助手';button.setAttribute('aria-expanded',String(!frame.hidden))});
  window.addEventListener('message',event=>{if(event.source===frame.contentWindow&&event.origin===origin&&event.data?.type==='mindcreek-assistant-ready-for-host')frame.contentWindow.postMessage({type:'mindcreek-assistant-host'},origin)});
  root.append(style,frame,button);document.body.append(container);
})();
