const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('frontend/assets/app.js','utf8');
const code=source.slice(source.indexOf('  let logoutPending=false;'),source.indexOf("  $('logout-btn').addEventListener"));
function fixture({confirmed=true,request=async()=>{}}={}){
  const controls={'logout-btn':{disabled:false},'account-modal-login':{disabled:false}};
  const state={account:{nickname:'example'},drive:{files:[{id:'file'}],selected:new Map([['file',{}]])}};
  const calls=[];
  const logout=vm.runInNewContext(`(()=>{${code};return logout})()`,{
    state,$:id=>controls[id],confirm:()=>confirmed,
    api:async(...args)=>{calls.push(args);await request()},
    account:async()=>{state.account=null},renderFiles:kind=>calls.push(['render',kind]),toast:message=>calls.push(['toast',message])
  });
  return {logout,state,calls,controls};
}
test('cancelling logout preserves account and files without sending DELETE',async()=>{
  const f=fixture({confirmed:false});assert.equal(await f.logout(),false);
  assert.ok(f.state.account);assert.equal(f.state.drive.files.length,1);assert.equal(f.calls.length,0);
});
test('successful logout deletes credentials and clears the visible account selection',async()=>{
  const f=fixture();assert.equal(await f.logout(),true);
  assert.equal(f.calls[0][0],'auth/account');assert.equal(f.calls[0][1],'DELETE');
  assert.equal(f.state.account,null);assert.equal(f.state.drive.files.length,0);assert.equal(f.state.drive.selected.size,0);
  assert.equal(f.controls['account-modal-login'].disabled,false);
});
test('failed logout keeps the account and re-enables both entry points',async()=>{
  const f=fixture({request:async()=>{throw Error('offline')}});await assert.rejects(f.logout(),/offline/);
  assert.ok(f.state.account);assert.equal(f.state.drive.files.length,1);
  assert.equal(f.controls['logout-btn'].disabled,false);assert.equal(f.controls['account-modal-login'].disabled,false);
});
test('repeated clicks while a logout request is pending issue only one DELETE',async()=>{
  let complete;const f=fixture({request:()=>new Promise(resolve=>{complete=resolve})});
  const pending=f.logout();assert.equal(await f.logout(),false);assert.equal(f.calls.length,1);
  complete();assert.equal(await pending,true);
});
