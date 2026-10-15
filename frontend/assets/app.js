(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'}[c]));
  const base = location.pathname.startsWith('/app/quarkdrive') ? '/app/quarkdrive' : '';
  const ico = name => `<svg aria-hidden="true"><use href="#i-${name}"></use></svg>`;
  const state = {
    page: 'drive', drive: {parent:'0', stack:[{id:'0',name:'全部文件'}], files:[], selected:new Map(), total:0, search:'',sort:'name',direction:1},
    share: {parent:'0', stack:[{id:'0',name:'分享文件'}], files:[], selected:new Map(), total:0, preview:null,sort:'name',direction:1},
    tasks:[], filter:'all', qr:null, poll:null, settings:null, locations:[], account:null,
    detailId:null, downloadKind:null, busy:false, settingsDirty:false, settingsSaving:false, settingsReady:false, settingsLoading:false, loginRequest:0, modal:null, modalTrigger:null, settingSection:'account-section', taskFresh:false, tasksRefreshing:false
  };
  const bytes = value => { const n = Number(value) || 0; if(n>=1073741824)return (n/1073741824).toFixed(2)+' GB'; if(n>=1048576)return (n/1048576).toFixed(1)+' MB'; if(n>=1024)return (n/1024).toFixed(1)+' KB'; return n+' B'; };
  const speed = n => bytes(Math.max(0,Number(n)||0))+'/s';
  const statuses = {QUEUED:'等待中',RESOLVING:'获取地址',DOWNLOADING:'下载中',VERIFYING:'正在校验',COMPLETED:'已完成',PAUSED:'已暂停',FAILED:'下载失败',BLOCKED:'等待恢复',RETRY_WAIT:'稍后重试'};
  const active = t => !['COMPLETED','FAILED','PAUSED','CANCELLED'].includes(t.status);
  const progress = t => Number(t.size)>0 ? Math.max(0, Math.min(100,100*Number(t.downloaded||0)/Number(t.size))) : t.status==='COMPLETED' ? 100 : 0;
  const fileKind = f => {if(f.folder)return 'folder';const ext=(f.name.split('.').at(-1)||'').toLowerCase();if(['jpg','png','jpeg','webp','gif','svg'].includes(ext))return 'image';if(['mp4','mkv','avi','mov','ts'].includes(ext))return 'video';if(['zip','rar','7z','tar','gz'].includes(ext))return 'archive';if(ext==='pdf')return 'pdf';if(['xls','xlsx','csv'].includes(ext))return 'sheet';return 'doc'};
  const time = iso => {if(!iso)return '—';const d=new Date(iso);return Number.isNaN(d.getTime())?'—':d.toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})};
  async function api(route, method='GET', body){
    const response = await fetch(base+'/api/v1/'+route,{method,headers:{'Content-Type':'application/json','X-QuarkDrive-Request':'1'},credentials:'same-origin',body:body===undefined?undefined:JSON.stringify(body)});
    let out;try {out=await response.json()}catch{throw Error('服务响应异常（HTTP '+response.status+'）')}
    if(!response.ok)throw Error(out.error || `请求失败（HTTP ${response.status}）`);
    return out;
  }
  function toast(message){const el=$('toast');el.textContent=String(message);el.classList.add('show');clearTimeout(el._timer);el._timer=setTimeout(()=>el.classList.remove('show'),3600)}
  async function run(callback){try{await callback()}catch(e){toast(e.message||String(e))}}
  const focusable = modal => [...modal.querySelectorAll('button,input,select,summary,a[href],[tabindex]')].filter(el=>!el.disabled&&el.tabIndex>=0&&el.getClientRects().length);
  function showModal(id){
    if(state.modal===id)return;
    state.modalTrigger=document.activeElement;state.modal=id;
    $(id).classList.remove('hidden');document.querySelector('.layout').inert=true;
    document.body.classList.add('modal-open');focusable($(id))[0]?.focus();
  }
  function hideModal(id){
    $(id).classList.add('hidden');if(state.modal!==id)return;
    state.modal=null;document.querySelector('.layout').inert=false;document.body.classList.remove('modal-open');
    if(state.modalTrigger?.isConnected&&state.modalTrigger.getClientRects().length)state.modalTrigger.focus();
    state.modalTrigger=null;
  }
  function page(name){
    state.page=name;
    for(const key of ['drive','share','tasks','settings'])$('page-'+key).classList.toggle('hidden',key!==name);
    document.querySelectorAll('.nav').forEach(el=>{const on=el.dataset.page===name;el.classList.toggle('active',on);el.setAttribute('aria-current',on?'page':'false')});
    if(name==='tasks')run(refreshTasks);
    if(name==='settings'&&!state.settingsReady)run(loadSettings);
    if(name==='drive'&&state.account&&!state.drive.files.length)run(()=>loadFiles('drive'));
  }
  document.querySelectorAll('.nav').forEach(el=>el.addEventListener('click',()=>page(el.dataset.page)));
  function fileRow(f,kind){const type=fileKind(f),s=state[kind],checked=s.selected.has(f.id);
    // Single-click opens folders; checkboxes stay independent of navigation.
    const folderAttrs = f.folder ? ` data-folder-kind="${kind}" data-folder-id="${esc(f.id)}" tabindex="0" title="单击打开文件夹（或按回车）"` : '';
    return `<tr class="${checked?'row-selected':''}"${folderAttrs}><td class="cbox"><input aria-label="选择 ${esc(f.name)}" type="checkbox" data-select="${kind}" data-id="${esc(f.id)}" ${checked?'checked':''}></td><td><div class="file-label"><span class="file-icon ${type}">${ico(f.folder?'folder':'file')}</span><span class="filename" title="${esc(f.name)}">${esc(f.name)}</span></div></td>${kind==='drive'?`<td class="file-type">${esc(time(f.modified_at))}</td>`:''}<td>${f.folder?'—':bytes(f.size)}</td><td class="file-type">${f.folder?'文件夹':type==='pdf'?'PDF':type==='video'?'视频':type==='image'?'图片':type==='archive'?'压缩包':type==='sheet'?'表格':'文件'}</td></tr>`;
  }
  // Rebuilding rows must not discard keyboard focus during selection or polling.
  function replaceContent(el,html){
    if(el.innerHTML===html)return;
    const focused=document.activeElement;
    const attrs=el.contains(focused)?['id','data-id','data-select','data-detail','data-task-action','data-go'].map(key=>[key,focused.getAttribute(key)]).filter(([,value])=>value!==null):[];
    el.innerHTML=html;
    if(attrs.length){
      const replacement=[...el.querySelectorAll('button,input,summary,[tabindex]')].find(item=>attrs.every(([key,value])=>item.getAttribute(key)===value));
      if(replacement&&!replacement.disabled)replacement.focus({preventScroll:true});
      else if(state.modal&&$(state.modal).contains(el))focusable($(state.modal))[0]?.focus();
    }
  }
  function emptyMarkup(title,description,buttons=''){
    return `<strong>${esc(title)}</strong><p>${esc(description)}</p>${buttons?`<div class="empty-actions">${buttons}</div>`:''}`;
  }
  function sortedFiles(s){
    return [...s.files].sort((a,b)=>{
      if(!!a.folder!==!!b.folder)return a.folder?-1:1;
      let result=s.sort==='name'?String(a.name).localeCompare(String(b.name),'zh-CN',{numeric:true}):Number(a[s.sort]||0)-Number(b[s.sort]||0);
      if(!result)result=String(a.name).localeCompare(String(b.name),'zh-CN',{numeric:true});
      return result*s.direction;
    });
  }
  function renderFiles(kind){
    const s=state[kind],count=s.files.length,unavailable=!!s.loading||!!s.error||(kind==='drive'&&!state.account)||(kind==='share'&&!s.preview);
    const hasFiles=count>0&&!unavailable;
    replaceContent($(kind+'-files'),hasFiles?sortedFiles(s).map(f=>fileRow(f,kind)).join(''):'');
    const wrap=$(kind+'-files').closest('.tablewrap'),empty=$(kind+'-empty');
    wrap.classList.toggle('is-empty',!hasFiles);wrap.setAttribute('aria-busy',s.loading?'true':'false');
    if(s.loading)empty.innerHTML=emptyMarkup('正在加载文件…',s.loadedCount?`已读取 ${s.loadedCount} 项${s.loadingTotal?' / 共 '+s.loadingTotal+' 项':''}`:'正在读取完整目录，请稍候');
    else if(kind==='drive'&&!state.account)empty.innerHTML=emptyMarkup('登录夸克，开始下载到 NAS','扫码连接账号后，即可浏览和选择网盘文件。','<button class="btn primary" data-login>扫码登录夸克</button>');
    else if(s.error)empty.innerHTML=emptyMarkup('文件加载失败',s.error,`<button class="btn secondary" data-reload="${kind}">重新加载</button>`);
    else if(kind==='share'&&!s.preview)empty.innerHTML=emptyMarkup('等待解析分享链接','在上方粘贴夸克分享链接，解析后选择要下载的文件。');
    else if(s.search&&!count)empty.innerHTML=emptyMarkup('没有找到相关文件','试试其他关键词，或清除搜索查看全部文件。','<button class="btn secondary" data-clear-search>清除搜索</button>');
    else if(!count)empty.innerHTML=emptyMarkup('当前目录没有文件','返回上一级查看其他目录。');
    $(kind+'-selected').textContent=s.selected.size?`已选择 ${s.selected.size} 项`:'选择文件后可下载';
    $(kind+'-all').disabled=!hasFiles;
    $(kind+'-all').checked=hasFiles&&s.files.every(f=>s.selected.has(f.id));
    $(kind+'-all').indeterminate=hasFiles&&s.files.some(f=>s.selected.has(f.id))&&!$(kind+'-all').checked;
    $(kind+'-download').disabled=unavailable||!s.selected.size;
    $(kind+'-selection').classList.toggle('hidden',!s.selected.size);
    document.querySelectorAll('#page-'+kind+' [data-sort]').forEach(button=>{
      const on=button.dataset.sort===s.sort;
      button.closest('th').setAttribute('aria-sort',on?(s.direction===1?'ascending':'descending'):'none');
      button.querySelector('.sort-indicator').textContent=on?(s.direction===1?' ↑':' ↓'):'';
      button.disabled=!hasFiles;
    });
    $(kind+'-crumbs').innerHTML=s.stack.map((item,index)=>`<button data-crumb="${kind}" data-pos="${index}">${esc(item.name)}</button>`).join('');
    if(kind==='drive'){
      $('drive-back').disabled=!!s.loading||s.stack.length<=1;
      $('drive-search').disabled=!state.account;$('drive-search-toggle').disabled=!state.account;
      $('drive-reload').disabled=!state.account||!!s.loading;
    }else{
      $('share-reload').disabled=!s.preview||!!s.loading;
      $('share-crumbs').classList.toggle('hidden',!s.preview);
      $('share-browser-toolbar').classList.toggle('hidden',!s.preview);
      $('share-selection-info').textContent=`${count} 项文件或文件夹`;
    }
  }
  async function loadFiles(kind, firstPage){
    const s=state[kind];if((kind==='share'&&!s.preview)||(kind==='drive'&&!state.account)){renderFiles(kind);return}
    const request=(s.request||0)+1;s.request=request;s.loading=true;s.error='';s.loadedCount=0;s.loadingTotal=0;renderFiles(kind);
    const parent=s.parent,search=s.search,previewId=s.preview?.id;
    const route=page=>kind==='drive'?(search?`files/search?q=${encodeURIComponent(search)}&page=${page}`:`files?parent=${encodeURIComponent(parent)}&page=${page}`):`shares/previews/${encodeURIComponent(previewId)}/files?parent=${encodeURIComponent(parent)}&page=${page}`;
    try{
      const files=await window.QuarkFilePages.collectPages(page=>api(route(page)),{
        firstPage,isCurrent:()=>s.request===request,
        onProgress:(count,total)=>{s.loadedCount=count;s.loadingTotal=total;renderFiles(kind)}
      });
      if(s.request!==request||files===null)return;
      s.files=files;s.total=files.length;
      const ids=new Set(files.map(file=>file.id));for(const id of s.selected.keys())if(!ids.has(id))s.selected.delete(id);
    }catch(e){if(s.request===request)s.error=e.message||String(e)}
    finally{if(s.request===request){s.loading=false;renderFiles(kind)}}
  }
  async function descend(kind,id){const s=state[kind],file=s.files.find(f=>f.id===id);if(!file||!file.folder)return;s.stack.push({id,name:file.name});s.parent=id;s.selected.clear();s.search='';if(kind==='drive')resetSearchControl();await loadFiles(kind)}
  async function crumb(kind,pos){const s=state[kind];s.stack=s.stack.slice(0,pos+1);s.parent=s.stack.at(-1).id;s.selected.clear();s.search='';if(kind==='drive')resetSearchControl();await loadFiles(kind)}
  async function searchDrive(){const s=state.drive;s.search=$('drive-search').value.trim();s.parent='0';s.selected.clear();s.stack=[{id:'0',name:s.search?'搜索结果':'全部文件'}];if(!s.search)expandSearch(false,true);await loadFiles('drive')}
  function expandSearch(open,focus=false){
    $('drive-search-field').classList.toggle('hidden',!open);
    $('drive-search-toggle').setAttribute('aria-expanded',String(open));
    if(focus)(open?$('drive-search'):$('drive-search-toggle')).focus();
  }
  function resetSearchControl(){
    $('drive-search').value='';$('drive-search-clear').classList.add('hidden');expandSearch(false);
  }
  $('drive-search-toggle').addEventListener('click',()=>{
    const open=$('drive-search-toggle').getAttribute('aria-expanded')==='true';
    expandSearch(!open||!!$('drive-search').value,true);
  });
  $('drive-search').addEventListener('keydown',e=>{
    if(e.key==='Enter')run(searchDrive);
    if(e.key==='Escape'){
      e.preventDefault();$('drive-search').value=state.drive.search;
      $('drive-search-clear').classList.toggle('hidden',!state.drive.search);
      if(!state.drive.search)expandSearch(false,true);else $('drive-search-toggle').focus();
    }
  });
  $('drive-search').addEventListener('input',e=>$('drive-search-clear').classList.toggle('hidden',!e.target.value));
  $('drive-search-control').addEventListener('focusout',e=>{
    if(!e.currentTarget.contains(e.relatedTarget)&&!$('drive-search').value&&!state.drive.search)expandSearch(false);
  });
  $('drive-search-clear').addEventListener('click',()=>{resetSearchControl();$('drive-search-toggle').focus();run(searchDrive)});
  $('drive-back').addEventListener('click',()=>run(()=>crumb('drive',Math.max(0,state.drive.stack.length-2))));
  $('drive-reload').addEventListener('click',()=>run(()=>loadFiles('drive')));
  $('share-reload').addEventListener('click',()=>run(()=>loadFiles('share')));
  for(const kind of ['drive','share']){
    $(kind+'-all').addEventListener('change',e=>{const s=state[kind];s.files.forEach(f=>e.target.checked?s.selected.set(f.id,f):s.selected.delete(f.id));renderFiles(kind)});
    $(kind+'-download').addEventListener('click',()=>run(()=>openDownload(kind)));
  }
  function shareError(message){
    $('share-error').textContent=message;$('share-error').classList.toggle('hidden',!message);
    $('share-url').setAttribute('aria-invalid',message?'true':'false');
  }
  $('share-url').addEventListener('input',()=>shareError(''));
  $('share-code').addEventListener('input',()=>shareError(''));
  async function parseShare(){
    const url=$('share-url').value.trim();
    if(!url){shareError('请输入夸克分享链接');$('share-url').focus();return}
    const btn=$('share-parse');if(btn.disabled)return;
    shareError('');btn.disabled=true;btn.textContent='解析中…';
    const s=state.share;s.request=(s.request||0)+1;s.loading=true;s.error='';s.preview=null;s.files=[];s.selected.clear();renderFiles('share');
    try{
      const out=await api('shares/previews','POST',{url,code:$('share-code').value.trim()});
      s.preview=out.preview;s.parent='0';
      s.stack=[{id:'0',name:out.preview?.title||'分享文件'}];await loadFiles('share',out.files);if(!s.error)toast('分享解析成功');
    }catch(e){shareError(e.message||String(e));$('share-url').focus()}
    finally{s.loading=false;renderFiles('share');btn.disabled=false;btn.textContent='解析链接'}
  }
  $('share-parse').addEventListener('click',parseShare);
  for(const id of ['share-url','share-code'])$(id).addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();parseShare()}});
  async function loadLocations(){const result=await api('storage/locations');state.locations=result.locations||[];return state.locations}
  function populateLocation(select,current){select.innerHTML=state.locations.length?state.locations.map(l=>{const label=(l.name&&l.name!==l.path)?`${esc(l.name)}（${esc(l.path)}）`:esc(l.path);return `<option value="${esc(l.path)}">${label}</option>`}).join(''):'<option value="">暂无授权目录</option>';if(current)select.value=current;}
  async function openDownload(kind){
    const s=state[kind];if(!s.selected.size)throw Error('请先选择文件或文件夹');state.downloadKind=kind;
    const [settings]=await Promise.all([state.settings?Promise.resolve(state.settings):api('settings'),loadLocations()]);state.settings=settings;
    populateLocation($('download-target'),settings.download_dir);
    $('download-count').textContent=`已选择 ${s.selected.size} 项 · 下载将保留原有目录结构`;
    showModal('download-modal');
  }
  async function createDownload(){
    const kind=state.downloadKind,s=state[kind];if(!s.selected.size)return;
    if(state.busy)return;state.busy=true;$('download-confirm').disabled=true;
    try{
      const target=$('download-target').value;
      if(!target||!state.locations.some(l=>l.path===target))throw Error('没有可用的 NAS 授权目录');
      const settings=state.settings||await api('settings');
      if(settings.download_dir!==target){state.settings=await api('settings','PUT',{...settings,download_dir:target});if(state.settingsReady&&!state.settingsDirty)populateLocation($('setting-dir'),target);}
      const selected=[...s.selected.values()];
      const out=await api('tasks','POST',{source_kind:kind==='drive'?'personal':'share',parent:s.parent,selected,preview_id:s.preview?.id||'',search:s.search||'',subdir:''});
      hideModal('download-modal');s.selected.clear();renderFiles(kind);toast(`创建 ${out.created} 个下载任务`);page('tasks');
    }finally{state.busy=false;$('download-confirm').disabled=false}
  }
  $('download-confirm').addEventListener('click',()=>run(createDownload));
  $('download-cancel').addEventListener('click',()=>hideModal('download-modal'));
  $('download-modal-close').addEventListener('click',()=>hideModal('download-modal'));
  function taskCounts(){const t=state.tasks;return {all:t.length,active:t.filter(active).length,completed:t.filter(x=>x.status==='COMPLETED').length,failed:t.filter(x=>x.status==='FAILED').length,paused:t.filter(x=>x.status==='PAUSED').length}}
  async function refreshTasks(){if(state.tasksRefreshing)return;state.tasksRefreshing=true;try{const out=await api('tasks');state.tasks=out.tasks||[];state.taskFresh=true;renderTasks();if(state.detailId&&!$('task-modal').classList.contains('hidden'))renderDetail()}catch(e){state.taskFresh=false;renderTasks();if(state.detailId&&!$('task-modal').classList.contains('hidden'))renderDetail();throw e}finally{state.tasksRefreshing=false}}
  function renderTasks(){
    const c=taskCounts();Object.entries(c).forEach(([key,value])=>{$('filter-'+key).textContent=value;$('filter-'+key).classList.toggle('hidden',value===0)});
    const running=state.tasks.filter(t=>t.status==='DOWNLOADING');
    $('queue-summary').classList.toggle('hidden',!state.tasks.length);
    $('queue-running').textContent=`正在下载 ${running.length} 项`;
    $('queue-speed').textContent=state.taskFresh?`总速度 ${speed(running.reduce((sum,t)=>sum+Math.max(0,Number(t.speed)||0),0))}`:'连接中断，等待更新';
    $('queue-waiting').textContent=`等待中 ${state.tasks.filter(t=>['QUEUED','RESOLVING','RETRY_WAIT','BLOCKED'].includes(t.status)).length} 项`;
    $('task-count').textContent=c.active;$('task-count').classList.toggle('hidden',!c.active);
    const f=state.filter;
    const filtered=state.tasks.filter(t=>f==='all'||(f==='active'?active(t):f==='failed'?t.status==='FAILED':f==='paused'?t.status==='PAUSED':t.status==='COMPLETED'));
    replaceContent($('task-list'),filtered.length?filtered.slice().reverse().map(t=>{
      const p=progress(t),status=t.status||'QUEUED',type=fileKind({name:t.name,folder:false});
      const buttons=active(t)?`<button class="icon-btn" title="暂停任务" aria-label="暂停 ${esc(t.name)}" data-task-action="pause" data-id="${esc(t.id)}">${ico('pause')}</button>`:status==='PAUSED'?`<button class="icon-btn" title="继续下载" aria-label="继续 ${esc(t.name)}" data-task-action="resume" data-id="${esc(t.id)}">${ico('play')}</button>`:status==='FAILED'?`<button class="icon-btn" title="重试任务" aria-label="重试 ${esc(t.name)}" data-task-action="retry" data-id="${esc(t.id)}">${ico('refresh')}</button>`:'<span class="action-placeholder" aria-hidden="true"></span>';
      return `<tr><td><div class="task-cell"><span class="file-icon ${type}">${ico('file')}</span><div class="minzero"><button class="task-name" title="${esc(t.relative||t.name)}" data-detail="${esc(t.id)}">${esc(t.name)}</button><span class="task-sub">${esc(t.kind==='share'?'分享下载':'我的网盘')}${t.relative&&t.relative!==t.name?' · '+esc(t.relative):''}</span>${t.status==='FAILED'&&t.message?`<span class="task-error" title="${esc(t.message)}">失败原因：${esc(t.message)}</span>`:''}</div></div></td><td>${bytes(t.size)}</td><td><div class="task-progress"><progress class="meter ${status==='COMPLETED'?'done':''}" value="${p.toFixed(2)}" max="100"></progress><span>${p.toFixed(0)}%</span></div></td><td>${status==='DOWNLOADING'&&state.taskFresh?speed(t.speed):'—'}</td><td><span class="status ${esc(status)}">${esc(statuses[status]||status)}</span></td><td><div class="task-actions">${buttons}<button class="icon-btn" title="查看详情" aria-label="查看任务详情" data-detail="${esc(t.id)}">${ico('more')}</button></div></td></tr>`;
    }).join(''):'');
    $('task-results').classList.toggle('is-empty',!filtered.length);
    $('tasks-empty').classList.toggle('hidden',!!filtered.length);
    const emptyHTML=state.tasks.length?emptyMarkup('暂无符合条件的下载任务','切换其他分类查看任务。'):emptyMarkup('还没有下载任务','从网盘或分享链接选择文件，创建第一个下载任务。','<button class="btn primary" data-go="drive">去我的网盘</button><button class="btn secondary" data-go="share">解析分享链接</button>');
    if($('tasks-empty').innerHTML!==emptyHTML)$('tasks-empty').innerHTML=emptyHTML;
  }
  function selectTaskTab(el){
    state.filter=el.dataset.filter;
    document.querySelectorAll('.tab').forEach(x=>{const on=x===el;x.classList.toggle('selected',on);x.setAttribute('aria-selected',String(on));x.tabIndex=on?0:-1});
    $('task-results').setAttribute('aria-labelledby',el.id);renderTasks();
  }
  document.querySelectorAll('.tab').forEach(el=>{
    el.addEventListener('click',()=>selectTaskTab(el));
    el.addEventListener('keydown',e=>{
      if(!['ArrowLeft','ArrowRight','Home','End'].includes(e.key))return;e.preventDefault();
      const tabs=[...document.querySelectorAll('.tab')],index=tabs.indexOf(el);
      const next=e.key==='Home'?0:e.key==='End'?tabs.length-1:(index+(e.key==='ArrowRight'?1:-1)+tabs.length)%tabs.length;
      tabs[next].focus();selectTaskTab(tabs[next]);
    });
  });
  $('task-refresh').addEventListener('click',()=>run(refreshTasks));
  function renderDetail(){
    const t=state.tasks.find(x=>x.id===state.detailId);if(!t){hideModal('task-modal');return}
    const p=progress(t),chunks=t.chunks||[],done=chunks.filter(x=>x.done).length;
    const expanded=$('task-diagnostics')?.open;
    const blocks=chunks.length?chunks.slice(0,480).map(x=>`<span class="chunk-cell ${x.done?'done':'pending'}" title="${x.done?'已完成':'未完成'}"></span>`).join(''):'<p class="chunk-caption">暂无分片信息</p>';
    const action=active(t)?`<button class="btn secondary" data-task-action="pause" data-id="${esc(t.id)}">${ico('pause')} 暂停下载</button>`:t.status==='PAUSED'?`<button class="btn primary" data-task-action="resume" data-id="${esc(t.id)}">${ico('play')} 继续下载</button>`:t.status==='FAILED'?`<button class="btn primary" data-task-action="retry" data-id="${esc(t.id)}">${ico('refresh')} 重试任务</button>`:'';
    replaceContent($('task-detail-content'),`<div class="detail-title"><span class="file-icon ${fileKind({name:t.name,folder:false})}">${ico('file')}</span><div><h2>${esc(t.name)}</h2><div class="detail-meta">${esc(t.kind==='share'?'分享下载':'我的网盘')}</div></div></div><div class="detail-progress-heading"><strong>${p.toFixed(1)}%</strong><span class="status ${esc(t.status)}">${esc(statuses[t.status]||t.status)}</span></div><progress class="meter detail-meter ${t.status==='COMPLETED'?'done':''}" value="${p.toFixed(2)}" max="100"></progress><div class="detail-stats"><div class="detail-stat"><small>已下载 / 总大小</small><strong>${bytes(t.downloaded)} / ${bytes(t.size)}</strong></div><div class="detail-stat"><small>下载速度</small><strong>${t.status==='DOWNLOADING'&&state.taskFresh?speed(t.speed):'—'}</strong></div></div><div class="detail-location"><small>保存位置</small><p>${esc(t.destination||'')} / ${esc(t.relative||t.name)}</p></div>${t.message?`<p class="detail-feedback">${esc(t.message)}</p>`:''}<div class="detail-actions">${action}</div><details class="advanced-settings" id="task-diagnostics" ${expanded?'open':''}><summary id="diagnostic-toggle">诊断信息</summary><p class="tiny-help">重试 ${Number(t.attempts||0)} 次 · 创建于 ${esc(time(t.created))}</p><h3>下载分片（${done}/${chunks.length}）</h3><div class="chunk-grid">${blocks}</div><p class="chunk-caption">绿色为已完成分片，蓝色为未完成分片；最多显示前 480 个。</p></details>`);
  }
  function openDetail(id){state.detailId=id;renderDetail();showModal('task-modal')}
  $('detail-close').addEventListener('click',()=>{state.detailId=null;hideModal('task-modal')});
  function folderRowFromEvent(e){
    if(e.target.closest('input,button,a,label'))return null;
    const row=e.target.closest('tr[data-folder-kind][data-folder-id]');
    return row && (row.closest('#drive-files') || row.closest('#share-files')) ? row : null;
  }
  // Ignore checkbox/control clicks so folders can still be selected for download.
  document.addEventListener('click',e=>{
    const row=folderRowFromEvent(e);
    if(row){e.preventDefault();run(()=>descend(row.dataset.folderKind,row.dataset.folderId))}
  });
  document.addEventListener('keydown',e=>{
    if(e.key!=='Enter' || e.target.tagName!=='TR')return;
    const row=folderRowFromEvent(e);
    if(row){e.preventDefault();run(()=>descend(row.dataset.folderKind,row.dataset.folderId))}
  });
  document.addEventListener('click',e=>{
    if(e.target.closest('[data-login]')){run(login);return}
    const go=e.target.closest('[data-go]');if(go){page(go.dataset.go);return}
    const reload=e.target.closest('[data-reload]');if(reload){run(()=>loadFiles(reload.dataset.reload));return}
    if(e.target.closest('[data-clear-search]')){resetSearchControl();$('drive-search-toggle').focus();run(searchDrive);return}
    const sort=e.target.closest('[data-sort]');if(sort){const kind=sort.closest('#page-drive')?'drive':'share',s=state[kind];s.direction=s.sort===sort.dataset.sort?-s.direction:1;s.sort=sort.dataset.sort;renderFiles(kind);return}
    const bread=e.target.closest('[data-crumb]');if(bread){run(()=>crumb(bread.dataset.crumb,Number(bread.dataset.pos)));return}
    const detail=e.target.closest('[data-detail]');if(detail){openDetail(detail.dataset.detail);return}
    const button=e.target.closest('[data-task-action]');if(button){run(async()=>{button.disabled=true;try{await api(`tasks/${encodeURIComponent(button.dataset.id)}/${encodeURIComponent(button.dataset.taskAction)}`,'POST');await refreshTasks()}finally{button.disabled=false}})}
  });
  document.addEventListener('change',e=>{const input=e.target;if(!input.dataset.select)return;const s=state[input.dataset.select];const f=s.files.find(v=>v.id===input.dataset.id);if(input.checked&&f)s.selected.set(f.id,f);else s.selected.delete(input.dataset.id);renderFiles(input.dataset.select)});
  async function account(){
    try{
      const out=await api('auth/account');state.account=out;
      $('account-mini').textContent=out.nickname||'夸克已登录';$('account-hint').textContent='已连接 · 点击查看';
      $('account-name').textContent=out.nickname||'夸克已登录';$('account-description').textContent=out.uid?'UID：'+out.uid:'已登录夸克网盘';
      updateAccountControls();
      return out;
    }catch{
      state.account=null;$('account-mini').textContent='未登录夸克';$('account-hint').textContent='点击扫码登录';
      $('account-name').textContent='未登录';$('account-description').textContent='登录后可访问个人网盘和分享下载';updateAccountControls();
      return null;
    }
  }
  function updateAccountControls(){
    $('logout-btn').classList.toggle('hidden',!state.account);
    $('account-login').classList.toggle('hidden',!!state.account);
    renderFiles('drive');
  }
  $('account-login').addEventListener('click',()=>run(login));
  function accountOpen(){
    if(!state.account){run(login);return}
    const info=state.account;
    $('account-modal-content').innerHTML=`<div class="account-detail"><div class="avatar">Q</div><h3>${esc(info.nickname||'夸克用户')}</h3><p>已登录 <span class="status COMPLETED">会话有效</span></p><small>${info.uid?'账号 ID：'+esc(info.uid):'账号信息由夸克接口提供'}</small></div>`;
    $('account-modal-login').textContent='重新扫码';showModal('account-modal');
  }
  $('account-open').addEventListener('click',accountOpen);
  $('account-modal-close').addEventListener('click',()=>hideModal('account-modal'));
  $('account-modal-login').addEventListener('click',()=>{hideModal('account-modal');run(login)});
  function qrCanvas(url){
    const qr=new window.QuarkQRCode(0,1);qr.addData(url);qr.make();
    const cv=$('qr-canvas'),ctx=cv.getContext('2d'),N=qr.getModuleCount(),cell=cv.width/(N+8);
    ctx.fillStyle='white';ctx.fillRect(0,0,256,256);ctx.fillStyle='#1c2c47';
    for(let y=0;y<N;y++)for(let x=0;x<N;x++)if(qr.isDark(y,x))ctx.fillRect(Math.floor((x+4)*cell),Math.floor((y+4)*cell),Math.ceil(cell),Math.ceil(cell));
  }
  async function login(){
    closeLogin();const request=++state.loginRequest;showModal('login-modal');$('qr-canvas').getContext('2d').clearRect(0,0,256,256);$('qr-status').textContent='正在获取二维码…';
    let out;try{out=await api('auth/qr-sessions','POST',{})}catch(e){if(request===state.loginRequest)$('qr-status').textContent='获取失败：'+e.message;return}
    if(request!==state.loginRequest)return;state.qr=out;qrCanvas(out.url);$('qr-status').textContent='等待扫码（二维码约 5 分钟有效）';
    state.poll=setInterval(async()=>{
      try{
        if(!state.qr)return;
        const result=await api('auth/qr-sessions/'+encodeURIComponent(state.qr.id));
        if(request!==state.loginRequest)return;
        $('qr-status').textContent={WAITING:'等待扫码…',SCANNED:'已扫码，请在手机上确认',EXPIRED:'二维码已过期，请刷新',SUCCESS:'登录成功'}[result.status]||result.status;
        if(result.status==='SUCCESS'){closeLogin();await account();run(()=>loadFiles('drive'));toast('扫码登录成功')}
        else if(['EXPIRED','FAILED','CANCELLED'].includes(result.status)){clearInterval(state.poll);state.poll=null}
      }catch(e){if(request===state.loginRequest)$('qr-status').textContent=e.message}
    },2500);
  }
  function closeLogin(){state.loginRequest++;clearInterval(state.poll);state.poll=null;state.qr=null;hideModal('login-modal')}
  $('modal-close').addEventListener('click',closeLogin);$('qr-retry').addEventListener('click',()=>run(login));
  $('account-check').addEventListener('click',()=>run(async()=>{const out=await account();toast(out?'账号登录状态正常':'尚未登录或登录已过期')}));
  $('logout-btn').addEventListener('click',()=>run(async()=>{if(!state.account)throw Error('当前尚未登录');if(!confirm('确定退出当前夸克账号？尚未完成的任务可能无法继续下载。'))return;await api('auth/account','DELETE');await account();state.drive.files=[];renderFiles('drive');toast('已退出登录')}));
  async function loadSettings(){
    if(state.settingsLoading||state.settingsDirty)return;state.settingsLoading=true;
    const fields=[...document.querySelectorAll('#downloads-section input,#storage-section select')];fields.forEach(el=>el.disabled=true);settingsStatus('正在加载下载设置…');
    try{
      const [settings]=await Promise.all([api('settings'),loadLocations()]);state.settings=settings;state.settingsReady=true;
      populateLocation($('setting-dir'),settings.download_dir);
      for(const key of ['concurrent','connections','chunk_mib','limit_mib','reserve_gib','retries'])$('setting-'+key).value=settings[key]??'';
      $('setting-cleanup').checked=!!settings.cleanup;syncThemeControls();settingsStatus('下载设置已保存');
    }catch(e){settingsStatus('设置加载失败，请重新进入设置页重试：'+(e.message||String(e)))}
    finally{state.settingsLoading=false;fields.forEach(el=>el.disabled=!state.settingsReady)}
  }
  function settingsStatus(message){$('settings-status').textContent=message;$('setting-save').disabled=!state.settingsReady||!state.settingsDirty||state.settingsSaving}
  function markSettingsDirty(){if(!state.settingsReady)return;state.settingsDirty=true;settingsStatus('有未保存的下载设置')}
  for(const key of ['concurrent','connections','chunk_mib','limit_mib','reserve_gib','retries','cleanup','dir'])$('setting-'+key).addEventListener('change',markSettingsDirty);
  for(const key of ['concurrent','connections','chunk_mib','limit_mib','reserve_gib','retries'])$('setting-'+key).addEventListener('input',markSettingsDirty);
  async function saveSettings(){
    const settings={...state.settings};
    if(state.settingsSaving||!state.settingsReady)return;
    for(const key of ['concurrent','connections','chunk_mib','limit_mib','reserve_gib','retries']){
      const el=$('setting-'+key),value=Number(el.value);
      if(!el.checkValidity() || !Number.isInteger(value) || value<0){settingSection('downloads-section');if(el.closest('details'))el.closest('details').open=true;el.focus();el.reportValidity();settingsStatus('请检查 '+el.parentElement.querySelector('span').textContent);return}
      settings[key]=value;
    }
    settings.cleanup=$('setting-cleanup').checked;settings.download_dir=$('setting-dir').value;
    if(!state.locations.some(l=>l.path===settings.download_dir)){settingSection('storage-section');$('setting-dir').focus();settingsStatus('请先选择 NAS 已授权下载目录');return}
    state.settingsSaving=true;settingsStatus('正在保存…');$('setting-save').textContent='保存中…';
    // Keep form values stable while this snapshot is being saved.
    const fields=[...document.querySelectorAll('#downloads-section input,#storage-section select')];fields.forEach(el=>el.disabled=true);
    try{state.settings=await api('settings','PUT',settings);state.settingsDirty=false;settingsStatus('下载设置已保存');toast('下载设置已保存')}
    catch(e){settingsStatus('保存失败：'+(e.message||String(e)))}
    finally{fields.forEach(el=>el.disabled=false);state.settingsSaving=false;settingsStatus($('settings-status').textContent);$('setting-save').textContent='保存下载设置'}
  }
  $('setting-save').addEventListener('click',()=>run(saveSettings));
  $('setting-dir-refresh').addEventListener('click',()=>run(async()=>{const current=$('setting-dir').value;await loadLocations();populateLocation($('setting-dir'),current);toast('已刷新授权目录')}));
  function settingSection(id){
    state.settingSection=id;
    document.querySelectorAll('.setting-panel').forEach(panel=>panel.classList.toggle('hidden',panel.id!==id));
    document.querySelectorAll('.setting-nav').forEach(button=>{const on=button.dataset.section===id;button.classList.toggle('active',on);button.setAttribute('aria-current',on?'page':'false')});
    $('settings-actions').classList.toggle('hidden',!['downloads-section','storage-section'].includes(id));
  }
  document.querySelectorAll('.setting-nav').forEach(btn=>btn.addEventListener('click',()=>settingSection(btn.dataset.section)));
  settingSection(state.settingSection);
  function syncThemeControls(){const preference=window.QuarkDriveTheme?.getPreference()||'auto';$('setting-theme').value=preference;document.querySelectorAll('input[name="theme-mode"]').forEach(i=>i.checked=i.value===preference)}
  document.querySelectorAll('input[name="theme-mode"]').forEach(el=>el.addEventListener('change',()=>{window.QuarkDriveTheme.setPreference(el.value);syncThemeControls()}));
  $('setting-theme').addEventListener('change',e=>{window.QuarkDriveTheme.setPreference(e.target.value);syncThemeControls()});
  document.addEventListener('keydown',e=>{
    if(!state.modal)return;
    if(e.key==='Escape'){e.preventDefault();if(state.modal==='login-modal')closeLogin();else hideModal(state.modal);return}
    if(e.key==='Tab'){
      const items=focusable($(state.modal)),first=items[0],last=items.at(-1);
      if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus()}
      else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus()}
    }
  });
  for(const name of ['login-modal','task-modal','account-modal','download-modal'])$(name).addEventListener('click',e=>{if(e.target!==$(name))return;if(name==='login-modal')closeLogin();else hideModal(name)});
  syncThemeControls();renderFiles('drive');renderFiles('share');
  setInterval(()=>{if(state.page==='tasks')run(refreshTasks)},1000);
  run(async()=>{ if(await account()) await loadFiles('drive'); });run(loadSettings);
})();
