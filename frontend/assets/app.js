(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'}[c]));
  const base = location.pathname.startsWith('/app/quarkdrive') ? '/app/quarkdrive' : '';
  const ico = name => `<svg aria-hidden="true"><use href="#i-${name}"></use></svg>`;
  const state = {
    page: 'drive', drive: {parent:'0', stack:[{id:'0',name:'全部文件'}], files:[], selected:new Map(), page:1, total:0, search:''},
    share: {parent:'0', stack:[{id:'0',name:'分享文件'}], files:[], selected:new Map(), page:1, total:0, preview:null},
    tasks:[], filter:'all', qr:null, poll:null, settings:null, locations:[], account:null,
    detailId:null, downloadKind:null, busy:false
  };
  const bytes = value => { const n = Number(value) || 0; if(n>=1073741824)return (n/1073741824).toFixed(2)+' GB'; if(n>=1048576)return (n/1048576).toFixed(1)+' MB'; if(n>=1024)return (n/1024).toFixed(1)+' KB'; return n+' B'; };
  const speed = n => Number(n)>0 ? bytes(n)+'/s' : '—';
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
  function showModal(id){$(id).classList.remove('hidden');}
  function hideModal(id){$(id).classList.add('hidden')}
  function page(name){
    state.page=name;
    for(const key of ['drive','share','tasks','settings'])$('page-'+key).classList.toggle('hidden',key!==name);
    document.querySelectorAll('.nav').forEach(el=>{const on=el.dataset.page===name;el.classList.toggle('active',on);el.setAttribute('aria-current',on?'page':'false')});
    if(name==='tasks')run(refreshTasks);
    if(name==='settings')run(loadSettings);
    if(name==='drive'&&!state.drive.files.length)run(()=>loadFiles('drive'));
  }
  document.querySelectorAll('.nav').forEach(el=>el.addEventListener('click',()=>page(el.dataset.page)));
  function fileRow(f,kind){const type=fileKind(f),s=state[kind],checked=s.selected.has(f.id);
    // Double-click folders just like a file manager. Checkboxes stay independent of navigation.
    const folderAttrs = f.folder ? ` data-folder-kind="${kind}" data-folder-id="${esc(f.id)}" tabindex="0" title="双击打开文件夹（或按回车）"` : '';
    return `<tr class="${checked?'row-selected':''}"${folderAttrs}><td class="cbox"><input aria-label="选择 ${esc(f.name)}" type="checkbox" data-select="${kind}" data-id="${esc(f.id)}" ${checked?'checked':''}></td><td><div class="file-label"><span class="file-icon ${type}">${ico(f.folder?'folder':'file')}</span><span class="filename" title="${esc(f.name)}">${esc(f.name)}</span></div></td>${kind==='drive'?`<td class="file-type">${esc(time(f.modified_at))}</td>`:''}<td>${f.folder?'—':bytes(f.size)}</td><td class="file-type">${f.folder?'文件夹':type==='pdf'?'PDF':type==='video'?'视频':type==='image'?'图片':type==='archive'?'压缩包':type==='sheet'?'表格':'文件'}</td></tr>`;
  }
  function renderFiles(kind){
    const s=state[kind];const count=s.files.length;
    $(kind+'-page').textContent=`第 ${s.page} 页${s.total?' · 共 '+s.total+' 项':''}`;
    $(kind+'-prev').disabled=s.page<=1;$(kind+'-next').disabled=count<100 || (s.total>0&&s.page*100>=s.total);
    $(kind+'-files').innerHTML=count?s.files.map(f=>fileRow(f,kind)).join(''):`<tr><td colspan="${kind==='drive'?5:4}" class="empty">${kind==='drive'&& !state.account?'登录夸克后可查看网盘文件':'当前目录没有文件'}</td></tr>`;
    $(kind+'-selected').textContent=`已选择 ${s.selected.size} 项`;
    $(kind+'-all').checked=count>0&&s.files.every(f=>s.selected.has(f.id));
    $(kind+'-all').indeterminate=count>0&&s.files.some(f=>s.selected.has(f.id))&&!$(kind+'-all').checked;
    $(kind+'-download').disabled=!s.selected.size;
    if(kind==='drive')$('drive-top-download').disabled=!s.selected.size;
    $(kind+'-crumbs').innerHTML=s.stack.map((item,index)=>`<button data-crumb="${kind}" data-pos="${index}">${esc(item.name)}</button>`).join('');
    if(kind==='share'){
      $('share-overview').classList.toggle('hidden',!s.preview);
      $('share-title').textContent=s.preview?.title || '分享文件';
      $('share-heading').textContent=s.preview?'分享内容预览':'分享内容预览';
      $('share-caption').textContent=s.preview?'选择要转存并下载的文件':'解析链接后显示文件目录';
      $('share-selection-info').textContent=`${s.total||s.files.length} 项文件或文件夹 · 已选 ${s.selected.size} 项`;
    }
  }
  async function loadFiles(kind){
    const s=state[kind];if(kind==='share'&&!s.preview)return;
    const route=kind==='drive'?(s.search?`files/search?q=${encodeURIComponent(s.search)}&page=${s.page}`:`files?parent=${encodeURIComponent(s.parent)}&page=${s.page}`):`shares/previews/${encodeURIComponent(s.preview.id)}/files?parent=${encodeURIComponent(s.parent)}&page=${s.page}`;
    const out=await api(route);s.files=out.files||[];s.total=out.total||0;renderFiles(kind);
  }
  async function descend(kind,id){const s=state[kind],file=s.files.find(f=>f.id===id);if(!file||!file.folder)return;s.stack.push({id,name:file.name});s.parent=id;s.page=1;s.selected.clear();s.search='';if(kind==='drive')$('drive-search').value='';await loadFiles(kind)}
  async function crumb(kind,pos){const s=state[kind];s.stack=s.stack.slice(0,pos+1);s.parent=s.stack.at(-1).id;s.page=1;s.selected.clear();s.search='';if(kind==='drive')$('drive-search').value='';await loadFiles(kind)}
  async function searchDrive(){const s=state.drive;s.search=$('drive-search').value.trim();s.parent='0';s.page=1;s.selected.clear();s.stack=[{id:'0',name:s.search?'搜索结果':'全部文件'}];await loadFiles('drive')}
  $('drive-search-btn').addEventListener('click',()=>run(searchDrive));
  $('drive-search').addEventListener('keydown',e=>{if(e.key==='Enter')run(searchDrive)});
  $('drive-search').addEventListener('input',e=>$('drive-search-clear').classList.toggle('hidden',!e.target.value));
  $('drive-search-clear').addEventListener('click',()=>{$('drive-search').value='';$('drive-search-clear').classList.add('hidden');run(searchDrive)});
  $('drive-back').addEventListener('click',()=>run(()=>crumb('drive',Math.max(0,state.drive.stack.length-2))));
  $('drive-reload').addEventListener('click',()=>run(()=>loadFiles('drive')));
  $('share-reload').addEventListener('click',()=>run(()=>loadFiles('share')));
  for(const kind of ['drive','share']){
    $(kind+'-all').addEventListener('change',e=>{const s=state[kind];s.files.forEach(f=>e.target.checked?s.selected.set(f.id,f):s.selected.delete(f.id));renderFiles(kind)});
    for(const [action,step] of [['prev',-1],['next',1]])$(kind+'-'+action).addEventListener('click',()=>run(async()=>{state[kind].page=Math.max(1,state[kind].page+step);await loadFiles(kind)}));
    $(kind+'-download').addEventListener('click',()=>run(()=>openDownload(kind)));
  }
  $('drive-top-download').addEventListener('click',()=>run(()=>openDownload('drive')));
  $('share-parse').addEventListener('click',()=>run(async()=>{
    const url=$('share-url').value.trim();if(!url)throw Error('请输入夸克分享链接');
    const btn=$('share-parse');btn.disabled=true;btn.textContent='解析中…';
    try{
      const out=await api('shares/previews','POST',{url,code:$('share-code').value.trim()});
      const s=state.share;s.preview=out.preview;s.files=out.files?.files||[];s.total=out.files?.total||0;s.parent='0';s.page=1;s.selected.clear();s.stack=[{id:'0',name:out.preview?.title||'分享文件'}];renderFiles('share');toast('分享解析成功');
    }finally{btn.disabled=false;btn.textContent='解析链接'}
  }));
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
      if(settings.download_dir!==target){state.settings=await api('settings','PUT',{...settings,download_dir:target});}
      const selected=[...s.selected.values()];
      const out=await api('tasks','POST',{source_kind:kind==='drive'?'personal':'share',parent:s.parent,selected,preview_id:s.preview?.id||'',search:s.search||'',subdir:''});
      hideModal('download-modal');s.selected.clear();renderFiles(kind);toast(`创建 ${out.created} 个下载任务`);page('tasks');
    }finally{state.busy=false;$('download-confirm').disabled=false}
  }
  $('download-confirm').addEventListener('click',()=>run(createDownload));
  $('download-cancel').addEventListener('click',()=>hideModal('download-modal'));
  $('download-modal-close').addEventListener('click',()=>hideModal('download-modal'));
  function taskCounts(){const t=state.tasks;return {all:t.length,active:t.filter(active).length,completed:t.filter(x=>x.status==='COMPLETED').length,failed:t.filter(x=>x.status==='FAILED').length,paused:t.filter(x=>x.status==='PAUSED').length}}
  async function refreshTasks(){const out=await api('tasks');state.tasks=out.tasks||[];renderTasks();if(state.detailId&&!$('task-modal').classList.contains('hidden'))renderDetail()}
  function renderTasks(){
    const c=taskCounts();Object.entries(c).forEach(([key,value])=>{$('filter-'+key).textContent=value});
    $('task-count').textContent=c.active;$('task-count').classList.toggle('hidden',!c.active);
    const f=state.filter;
    const filtered=state.tasks.filter(t=>f==='all'||(f==='active'?active(t):f==='failed'?t.status==='FAILED':f==='paused'?t.status==='PAUSED':t.status==='COMPLETED'));
    $('task-list').innerHTML=filtered.length?filtered.slice().reverse().map(t=>{
      const p=progress(t),status=t.status||'QUEUED',type=fileKind({name:t.name,folder:false});
      const buttons=active(t)?`<button class="icon-btn" title="暂停任务" aria-label="暂停 ${esc(t.name)}" data-task-action="pause" data-id="${esc(t.id)}">${ico('pause')}</button>`:status==='PAUSED'?`<button class="icon-btn" title="继续下载" aria-label="继续 ${esc(t.name)}" data-task-action="resume" data-id="${esc(t.id)}">${ico('play')}</button>`:status==='FAILED'?`<button class="icon-btn" title="重试任务" aria-label="重试 ${esc(t.name)}" data-task-action="retry" data-id="${esc(t.id)}">${ico('refresh')}</button>`:'';
      return `<tr><td><div class="task-cell"><span class="file-icon ${type}">${ico('file')}</span><div class="minzero"><button class="task-name" title="${esc(t.relative||t.name)}" data-detail="${esc(t.id)}">${esc(t.name)}</button><span class="task-sub">${esc(t.kind==='share'?'分享下载':'我的网盘')} · ${esc(t.relative||'')}</span>${t.status==='FAILED'&&t.message?`<span class="task-error" title="${esc(t.message)}">失败原因：${esc(t.message)}</span>`:''}</div></div></td><td>${bytes(t.size)}</td><td><div class="task-progress"><progress class="meter ${status==='COMPLETED'?'done':''}" value="${p.toFixed(2)}" max="100"></progress><span>${p.toFixed(0)}%</span></div></td><td>${status==='DOWNLOADING'?speed(t.speed):'—'}</td><td><span class="status ${esc(status)}">${esc(statuses[status]||status)}</span></td><td><div class="task-actions">${buttons}<button class="icon-btn" title="查看详情" aria-label="查看任务详情" data-detail="${esc(t.id)}">${ico('more')}</button></div></td></tr>`;
    }).join(''):'<tr><td colspan="6" class="empty">暂无符合条件的下载任务</td></tr>';
  }
  document.querySelectorAll('.tab').forEach(el=>el.addEventListener('click',()=>{state.filter=el.dataset.filter;document.querySelectorAll('.tab').forEach(x=>{x.classList.toggle('selected',x===el);x.setAttribute('aria-selected',x===el?'true':'false')});renderTasks()}));
  $('task-refresh').addEventListener('click',()=>run(refreshTasks));
  function renderDetail(){
    const t=state.tasks.find(x=>x.id===state.detailId);if(!t){hideModal('task-modal');return}
    const p=progress(t),chunks=t.chunks||[];const done=chunks.filter(x=>x.done).length;
    const blocks=chunks.length?chunks.slice(0,480).map(x=>`<span class="chunk-cell ${x.done?'done':'pending'}" title="${x.done?'已完成':'未完成'}"></span>`).join(''):'<span class="chunk-caption">暂无分片信息；任务开始后显示下载状态。</span>';
    const action=active(t)?`<button class="btn secondary" data-task-action="pause" data-id="${esc(t.id)}">${ico('pause')} 暂停下载</button>`:t.status==='PAUSED'?`<button class="btn primary" data-task-action="resume" data-id="${esc(t.id)}">${ico('play')} 继续下载</button>`:t.status==='FAILED'?`<button class="btn primary" data-task-action="retry" data-id="${esc(t.id)}">${ico('refresh')} 重试任务</button>`:'';
    $('task-detail-content').innerHTML=`<div class="detail-title"><span class="file-icon ${fileKind({name:t.name,folder:false})}">${ico('file')}</span><div><h2>${esc(t.name)}</h2><div class="detail-meta">${esc(t.relative||'')} · ${esc(t.kind==='share'?'分享链接':'我的网盘')}</div></div></div><div class="detail-progress-heading"><strong>${p.toFixed(1)}%</strong><span class="status ${esc(t.status)}">${esc(statuses[t.status]||t.status)}</span></div><progress class="meter detail-meter ${t.status==='COMPLETED'?'done':''}" value="${p.toFixed(2)}" max="100"></progress><div class="detail-stats"><div class="detail-stat"><small>文件大小</small><strong>${bytes(t.size)}</strong></div><div class="detail-stat"><small>已下载</small><strong>${bytes(t.downloaded)}</strong></div><div class="detail-stat"><small>下载速度</small><strong>${speed(t.speed)}</strong></div><div class="detail-stat"><small>重试次数</small><strong>${Number(t.attempts||0)}</strong></div></div><div class="detail-actions">${action}</div><div class="detail-section"><h3>下载分片 ${chunks.length?`（${done}/${chunks.length}）`:''}</h3><div class="chunk-grid">${blocks}</div>${chunks.length?'<p class="chunk-caption">绿色为已完成分片，蓝色为待恢复或下载中的分片；最多显示前 480 个。</p>':''}</div><div class="detail-section"><h3>任务信息</h3><div class="detail-message">保存位置：${esc(t.destination||'')} / ${esc(t.relative||'')}\n创建时间：${esc(time(t.created))}\n${t.message?'状态说明：'+esc(t.message):'状态说明：暂无错误'}</div></div>`;
  }
  function openDetail(id){state.detailId=id;renderDetail();showModal('task-modal')}
  $('detail-close').addEventListener('click',()=>{state.detailId=null;hideModal('task-modal')});
  function folderRowFromEvent(e){
    if(e.target.closest('input,button,a,label'))return null;
    const row=e.target.closest('tr[data-folder-kind][data-folder-id]');
    return row && (row.closest('#drive-files') || row.closest('#share-files')) ? row : null;
  }
  // Do not navigate on single click: it must remain possible to select a folder for download.
  document.addEventListener('dblclick',e=>{
    const row=folderRowFromEvent(e);
    if(row){e.preventDefault();run(()=>descend(row.dataset.folderKind,row.dataset.folderId))}
  });
  document.addEventListener('keydown',e=>{
    if(e.key!=='Enter' || e.target.tagName!=='TR')return;
    const row=folderRowFromEvent(e);
    if(row){e.preventDefault();run(()=>descend(row.dataset.folderKind,row.dataset.folderId))}
  });
  document.addEventListener('click',e=>{
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
      $('login-btn').textContent='账号信息';
      return out;
    }catch{
      state.account=null;$('account-mini').textContent='未登录夸克';$('account-hint').textContent='点击扫码登录';
      $('account-name').textContent='未登录';$('account-description').textContent='登录后可访问个人网盘和分享下载';$('login-btn').textContent='账号信息';
      return null;
    }
  }
  function accountOpen(){
    if(!state.account){
      $('account-modal-content').innerHTML=`<div class="account-detail"><div class="avatar">Q</div><h3>未登录夸克</h3><p>登录后可访问个人网盘和分享下载</p><small>登录凭据仅保存在当前 NAS 上，并会加密存储。</small></div>`;
      $('account-modal-login').textContent='扫码登录';showModal('account-modal');
      return;
    }
    const info=state.account;
    $('account-modal-content').innerHTML=`<div class="account-detail"><div class="avatar">Q</div><h3>${esc(info.nickname||'夸克用户')}</h3><p>已登录 <span class="status COMPLETED">会话有效</span></p><small>${info.uid?'账号 ID：'+esc(info.uid):'账号信息由夸克接口提供'}</small></div>`;
    $('account-modal-login').textContent='重新扫码';showModal('account-modal');
  }
  $('account-open').addEventListener('click',accountOpen);
  $('login-btn').addEventListener('click',accountOpen);
  $('account-modal-close').addEventListener('click',()=>hideModal('account-modal'));
  $('account-modal-login').addEventListener('click',()=>{hideModal('account-modal');run(login)});
  function qrCanvas(url){
    const qr=new window.QuarkQRCode(0,1);qr.addData(url);qr.make();
    const cv=$('qr-canvas'),ctx=cv.getContext('2d'),N=qr.getModuleCount(),cell=cv.width/(N+8);
    ctx.fillStyle='white';ctx.fillRect(0,0,256,256);ctx.fillStyle='#1c2c47';
    for(let y=0;y<N;y++)for(let x=0;x<N;x++)if(qr.isDark(y,x))ctx.fillRect(Math.floor((x+4)*cell),Math.floor((y+4)*cell),Math.ceil(cell),Math.ceil(cell));
  }
  async function login(){
    closeLogin();showModal('login-modal');$('qr-status').textContent='正在获取二维码…';
    let out;try{out=await api('auth/qr-sessions','POST',{})}catch(e){$('qr-status').textContent='获取失败：'+e.message;throw e}
    state.qr=out;qrCanvas(out.url);$('qr-status').textContent='等待扫码（二维码约 5 分钟有效）';
    state.poll=setInterval(async()=>{
      try{
        if(!state.qr)return;
        const result=await api('auth/qr-sessions/'+encodeURIComponent(state.qr.id));
        $('qr-status').textContent={WAITING:'等待扫码…',SCANNED:'已扫码，请在手机上确认',EXPIRED:'二维码已过期，请刷新',SUCCESS:'登录成功'}[result.status]||result.status;
        if(result.status==='SUCCESS'){closeLogin();await account();run(()=>loadFiles('drive'));toast('扫码登录成功')}
        else if(['EXPIRED','FAILED','CANCELLED'].includes(result.status)){clearInterval(state.poll);state.poll=null}
      }catch(e){$('qr-status').textContent=e.message}
    },2500);
  }
  function closeLogin(){clearInterval(state.poll);state.poll=null;state.qr=null;hideModal('login-modal')}
  $('modal-close').addEventListener('click',closeLogin);$('qr-retry').addEventListener('click',()=>run(login));
  $('account-check').addEventListener('click',()=>run(async()=>{const out=await account();toast(out?'账号登录状态正常':'尚未登录或登录已过期')}));
  $('logout-btn').addEventListener('click',()=>run(async()=>{if(!state.account)throw Error('当前尚未登录');if(!confirm('确定退出当前夸克账号？尚未完成的任务可能无法继续下载。'))return;await api('auth/account','DELETE');await account();state.drive.files=[];renderFiles('drive');toast('已退出登录')}));
  async function loadSettings(){
    const [settings]=await Promise.all([api('settings'),loadLocations()]);state.settings=settings;
    populateLocation($('setting-dir'),settings.download_dir);
    for(const key of ['concurrent','connections','chunk_mib','limit_mib','reserve_gib','retries'])$('setting-'+key).value=settings[key]??'';
    $('setting-cleanup').checked=!!settings.cleanup;
    syncThemeControls();
  }
  async function saveSettings(){
    const settings={...state.settings};
    if(!settings.download_dir)throw Error('请先选择 NAS 已授权下载目录');
    for(const key of ['concurrent','connections','chunk_mib','limit_mib','reserve_gib','retries']){
      const el=$('setting-'+key),value=Number(el.value);
      if(!el.checkValidity() || !Number.isInteger(value) || value<0)throw Error('请检查 '+el.parentElement.querySelector('span').textContent);
      settings[key]=value;
    }
    settings.cleanup=$('setting-cleanup').checked;settings.download_dir=$('setting-dir').value;
    if(!state.locations.some(l=>l.path===settings.download_dir))throw Error('下载目录未经授权');
    state.settings=await api('settings','PUT',settings);toast('下载设置已保存');
  }
  $('setting-save').addEventListener('click',()=>run(saveSettings));
  $('setting-dir-refresh').addEventListener('click',()=>run(async()=>{const current=$('setting-dir').value;await loadLocations();populateLocation($('setting-dir'),current);toast('已刷新授权目录')}));
  document.querySelectorAll('.setting-nav').forEach(btn=>btn.addEventListener('click',()=>{
    document.querySelectorAll('.setting-nav').forEach(el=>el.classList.toggle('active',el===btn));
    $(btn.dataset.section).scrollIntoView({behavior:'smooth',block:'start'});
  }));
  function syncThemeControls(){const preference=window.QuarkDriveTheme?.getPreference()||'auto';$('setting-theme').value=preference;document.querySelectorAll('input[name="theme-mode"]').forEach(i=>i.checked=i.value===preference)}
  document.querySelectorAll('input[name="theme-mode"]').forEach(el=>el.addEventListener('change',()=>{window.QuarkDriveTheme.setPreference(el.value);syncThemeControls()}));
  $('setting-theme').addEventListener('change',e=>{window.QuarkDriveTheme.setPreference(e.target.value);syncThemeControls()});
  document.addEventListener('keydown',e=>{if(e.key==='Escape'){if(!$('login-modal').classList.contains('hidden'))closeLogin();for(const name of ['task-modal','account-modal','download-modal'])hideModal(name)}});
  for(const name of ['login-modal','task-modal','account-modal','download-modal'])$(name).addEventListener('click',e=>{if(e.target!==$(name))return;if(name==='login-modal')closeLogin();else hideModal(name)});
  syncThemeControls();renderFiles('drive');renderFiles('share');
  setInterval(()=>{if(state.page==='tasks')run(refreshTasks)},2500);
  run(async()=>{ if(await account()) await loadFiles('drive'); });run(loadSettings);
})();
