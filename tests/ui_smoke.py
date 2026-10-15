"""Browser smoke tests using mocked HTTP responses; no real Quark account involved.
Run: python tests/ui_smoke.py --screenshots (requires playwright and Chromium).
"""
import json, os, sys, re
from pathlib import Path
from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18088/'
ROOT = Path(__file__).resolve().parents[1]
F = lambda name, id, folder=False, size=0: dict(id=id, name=name, folder=folder, size=size, parent='0', modified_at=1791961800000)
files = [F('我的备份','f1',True),F('影视资源','f2',True),F('学习资料','f3',True),F('手机照片','f4',True),F('夸克精选壁纸.jpg','a1',False,2400000),F('科目一速记口诀.pdf','a2',False,8700000),F('夸克云解压.zip','a3',False,1250000000),F('学习计划表.xlsx','a4',False,550000)]
share_files = [F('课程视频合集','s1',True),F('模拟试题.pdf','s2',False,8700000),F('重点笔记.docx','s3',False,12000000),F('分辨视频.mp4','s4',False,256000000),F('学习资料合集.zip','s5',False,1200000000)]
tasks = [
 dict(id='t1',name='科目一速记口诀.pdf',relative='科目一速记口诀.pdf',kind='personal',size=4800000000,downloaded=2100000000,speed=26300000,status='DOWNLOADING',created='2026-10-14T09:42:00Z',updated='2026-10-14T10:42:00Z',destination='/vol1/下载/夸克网盘',attempts=0,chunks=[dict(done=i<16,start=i*8000000,end=(i+1)*8000000) for i in range(36)]),
 dict(id='t2',name='夸克精选壁纸.jpg',relative='壁纸/夸克精选壁纸.jpg',kind='share',size=1200000000,downloaded=340000000,speed=16800000,status='DOWNLOADING',created='2026-10-14T09:42:00Z',destination='/vol1/下载/夸克网盘',attempts=0),
 dict(id='t3',name='学习资料合集.zip',relative='学习资料合集.zip',kind='share',size=8700000000,downloaded=8700000000,speed=0,status='COMPLETED',created='2026-10-14T09:42:00Z',destination='/vol1/下载/夸克网盘',attempts=0),
 dict(id='t4',name='考试资料包.zip',relative='考试资料包.zip',kind='personal',size=870000000,downloaded=0,speed=0,status='FAILED',message='网络连接被中断',created='2026-10-14T09:42:00Z',destination='/vol1/下载/夸克网盘',attempts=3),
 dict(id='t5',name='手机照片',relative='手机照片',kind='personal',size=2400000000,downloaded=375000000,speed=0,status='PAUSED',created='2026-10-14T09:42:00Z',destination='/vol1/下载/夸克网盘',attempts=0),
]
settings=dict(download_dir='/vol1/下载/夸克网盘',concurrent=3,connections=4,chunk_mib=8,limit_mib=0,reserve_gib=20,retries=5,cleanup=True)

def mock(route):
    url=route.request.url; path=url.split('/api/v1/',1)[-1]
    if path.startswith('auth/account'):
        return {'nickname':'夸克用户','uid':'188****1234','authenticated':True}
    if path.startswith('files/search'):
        return {'files':files[:4],'total':4,'page':1}
    if path.startswith('files'):
        return {'files':files,'total':len(files),'page':1}
    if path=='shares/previews':
        return {'preview':{'id':'p123','title':'科目一速记口诀 课程资料'},'files':{'files':share_files,'total':len(share_files),'page':1}}
    if path.startswith('shares/previews/'):
        return {'files':share_files,'total':len(share_files),'page':1}
    if path=='tasks':return {'tasks':tasks} if route.request.method=='GET' else {'created':1}
    if path.startswith('tasks/'):
        return next((t for t in tasks if path.split('/')[1]==t['id']),{})
    if path=='settings':return settings
    if path=='storage/locations':return {'locations':[{'id':'0','name':'夸克网盘','path':'/vol1/下载/夸克网盘'}]}
    if path=='auth/qr-sessions':return {'id':'demo','url':'https://su.quark.cn/test','status':'WAITING'}
    if path.startswith('auth/qr-sessions/'):return {'status':'WAITING'}
    return {'error':'Not mocked: '+path}

def run(screenshots=False):
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path='/usr/bin/chromium',args=['--no-sandbox','--disable-dev-shm-usage'])
        context=browser.new_context(viewport={'width':1440,'height':900},device_scale_factor=1)
        page=context.new_page(); errors=[]
        page.on('pageerror', lambda e:errors.append(str(e)))
        page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
        html=(ROOT/'frontend'/'index.html').read_text()
        html=re.sub(r'<script src=\"[^\"]+\"></script>', '', html)
        html=re.sub(r'<link rel=\"stylesheet\"[^>]*>', '', html)
        html=html.replace('</head>', '<style>'+ (ROOT/'frontend'/'assets'/'app.css').read_text() +'</style></head>')
        page.set_content(html, wait_until='load')
        page.add_script_tag(content=(ROOT/'frontend'/'assets'/'theme.js').read_text())
        page.add_script_tag(content=(ROOT/'frontend'/'assets'/'qr.js').read_text())
        # Mock fetch strictly in this test document; the shipped UI has no mock data.
        samples={'account':{'nickname':'夸克用户','uid':'188****1234','authenticated':True},'files':files,'share_files':share_files,'tasks':tasks,'settings':settings}
        page.evaluate('''(samples) => { window.__mockAccount=samples.account; window.fetch = async (url, options={}) => {
          const route = String(url).split('/api/v1/').pop(); let data={};
          if(route==='auth/account') {
            if(options.method==='DELETE'){window.__mockAccount=null;data={ok:true};}
            else if(window.__mockAccount) data=window.__mockAccount;
            else return new Response(JSON.stringify({error:'未登录'}), {status:401,headers:{'Content-Type':'application/json'}});
          }
          else if(route.startsWith('files/search')) data={files:samples.files.slice(0,4),total:4};
          else if(route.startsWith('files')) data={files:samples.files,total:samples.files.length};
          else if(route==='shares/previews') data={preview:{id:'p123',title:'科目一速记口诀 课程资料'},files:{files:samples.share_files,total:samples.share_files.length}};
          else if(route.startsWith('shares/previews/')) data={files:samples.share_files,total:samples.share_files.length};
          else if(route==='tasks') data=options.method==='POST'?{created:1}:{tasks:samples.tasks};
          else if(route.startsWith('tasks/')) data=samples.tasks.find(t=>t.id===route.split('/')[1])||{ok:true};
          else if(route==='settings') data=samples.settings;
          else if(route==='storage/locations') data={locations:[{id:'0',name:'夸克网盘',path:'/vol1/下载/夸克网盘'}]};
          else if(route==='auth/qr-sessions') data={id:'demo',url:'https://su.quark.cn/test',status:'WAITING'};
          else if(route.startsWith('auth/qr-sessions/')) data={status:'WAITING'};
          return new Response(JSON.stringify(data), {status:200,headers:{'Content-Type':'application/json'}});
        };}''', samples)
        page.add_script_tag(content=(ROOT/'frontend'/'assets'/'file-pages.js').read_text())
        page.add_script_tag(content=(ROOT/'frontend'/'assets'/'app.js').read_text())
        page.wait_for_timeout(250)
        assert page.locator('#drive-files tr').count()==len(files),'file list not rendered'
        assert page.locator('#drive-files [data-open]').count()==0
        assert page.locator('#drive-files th').count()==0  # headers live in thead, not tbody
        assert page.locator('#page-drive thead th').count()==5
        assert page.locator('#drive-search-btn').count()==0
        assert page.locator('.pagination,.file-footer').count()==0
        assert not page.locator('#drive-download').is_visible()
        page.locator('#page-drive [data-sort=size]').click()
        assert page.locator('#page-drive th[aria-sort=ascending]').inner_text().strip().startswith('大小')
        assert page.locator('#drive-files tr').first.get_attribute('data-folder-id') is not None
        # Single-click and Enter open folders; checkboxes only select.
        row=page.locator('#drive-files tr[data-folder-id="f1"]')
        row.locator('input[type=checkbox]').check()
        assert '我的备份' not in page.locator('#drive-crumbs').inner_text()
        row.locator('.filename').click(); page.wait_for_timeout(60)
        assert '我的备份' in page.locator('#drive-crumbs').inner_text(),'folder single-click did not navigate'
        page.locator('#drive-back').click(); page.wait_for_timeout(60)
        assert '我的备份' not in page.locator('#drive-crumbs').inner_text()
        page.locator('#drive-files tr[data-folder-id="f2"]').focus()
        page.keyboard.press('Enter');page.wait_for_timeout(60)
        assert '影视资源' in page.locator('#drive-crumbs').inner_text(),'Enter on folder did not navigate'
        page.locator('#drive-back').click();page.wait_for_timeout(60)
        out=ROOT/'dist'/'ui-preview';out.mkdir(parents=True,exist_ok=True)
        def snap(n):
            page.evaluate("document.querySelector('#toast').classList.remove('show')")
            if screenshots:page.screenshot(path=str(out/n),full_page=True)
        snap('01-drive-light.png')
        page.locator('#drive-files input[type=checkbox]').first.check()
        assert '已选择 1 项' in page.locator('#drive-selected').inner_text()
        page.locator('#drive-download').click()
        page.locator('#download-modal').wait_for(state='visible')
        page.locator('#download-cancel').click()
        page.locator('[data-page=share]').click();page.locator('#share-url').fill('https://pan.quark.cn/s/demo');page.locator('#share-parse').click()
        page.wait_for_timeout(250)
        assert page.locator('#share-files tr').count()==len(share_files),'share preview missing'
        assert page.locator('#page-share thead th').count()==4
        page.locator('#share-files tr[data-folder-id="s1"]').click(); page.wait_for_timeout(60)
        assert '课程视频合集' in page.locator('#share-crumbs').inner_text(),'share folder single-click did not navigate'
        page.locator('#share-crumbs button').first.click(); page.wait_for_timeout(60)
        assert '课程视频合集' not in page.locator('#share-crumbs').inner_text()
        snap('02-share-light.png')
        page.locator('[data-page=tasks]').click();page.wait_for_timeout(200)
        assert page.locator('#task-list tr').count()==len(tasks),'tasks missing'
        assert '总速度' in page.locator('#queue-speed').inner_text()
        snap('03-tasks-light.png')
        page.locator('[data-detail=t1]').first.click();page.locator('#task-modal').wait_for(state='visible')
        assert not page.locator('#task-diagnostics').evaluate('(el) => el.open')
        snap('04-task-detail.png');page.locator('#detail-close').click()
        page.locator('[data-page=settings]').click();page.wait_for_timeout(200);snap('05-settings-light.png')
        page.locator('[data-section=appearance-section]').click()
        page.locator('input[name=theme-mode][value=dark]').check();assert page.locator('html').get_attribute('data-theme')=='dark'
        page.locator('[data-page=drive]').click();snap('06-drive-dark.png')
        page.locator('#account-open').click();snap('07-account.png');page.locator('#account-modal-close').click()
        page.locator('[data-page=settings]').click()
        page.on('dialog',lambda dialog:dialog.accept())
        page.locator('[data-section=account-section]').click()
        page.locator('#logout-btn').click()
        page.locator('#account-open').click()
        page.locator('#login-modal').wait_for(state='visible')
        snap('08-qr-login.png')
        page.locator('#modal-close').click()
        page.set_viewport_size({'width':770,'height':850})
        snap('09-compact.png')
        assert not errors,'Browser errors:\n'+'\n'.join(errors)
        print('UI smoke pass; screenshots at',out)
        browser.close()
if __name__=='__main__': run('--screenshots' in sys.argv)
