const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../frontend/assets/theme.js'), 'utf8');
function target() {
  const listeners = new Map();
  return {
    addEventListener(type, callback) { if (!listeners.has(type)) listeners.set(type, new Set()); listeners.get(type).add(callback); },
    removeEventListener(type, callback) { listeners.get(type)?.delete(callback); },
    dispatchEvent(event) { for (const callback of [...(listeners.get(event.type) || [])]) callback(event); },
    listenerCount(type) { return listeners.get(type)?.size || 0; }
  };
}
function deferred() { let resolve, reject; const promise = new Promise((a,b) => { resolve=a; reject=b; }); return {promise,resolve,reject}; }
function setup({darkMedia=false,local={}}={}) {
  const attrs = new Map(), store = new Map(Object.entries(local));
  const media = Object.assign(target(), {matches:darkMedia});
  const window = Object.assign(target(), {matchMedia:() => media,localStorage:{getItem:k=>store.get(k)??null,setItem:(k,v)=>store.set(k,v)}});
  // Host DOM and storage must never be probed, even when same-origin.
  Object.defineProperty(window,'parent',{get(){throw Error('parent probing is forbidden');}});
  const root={setAttribute:(k,v)=>attrs.set(k,v),getAttribute:k=>attrs.get(k),style:{}};
  const document={documentElement:root,getElementById:()=>null};
  vm.runInNewContext(code,{window,document,Event:class {constructor(type){this.type=type;}}});
  return {theme:window.QuarkDriveTheme,window,media,root,store};
}
function host(theme='light',options={}) {
  const callbacks = new Set(),off=[];
  const sdk={isWeb:true,isStandaloneWeb:false,ready:async()=>{},getPlatformConfig:async()=>({theme}),
    $on:async(event,callback)=>{assert.equal(event,'os/theme');callbacks.add(callback);},
    $off:async(event,callback)=>{assert.equal(event,'os/theme');off.push(callback);callbacks.delete(callback);},...options};
  return {sdk,off,callbacks,emit:theme=>{for(const cb of callbacks)cb(theme);}};
}
test('fnOS light wins over a dark browser and later browser changes',async()=>{
  const x=setup({darkMedia:true}),h=host('light');assert.equal(x.theme.getResolved(),'dark');
  await x.theme.connectHost(h.sdk);assert.equal(x.theme.getResolved(),'light');assert.equal(x.theme.getSource(),'fnos');
  x.media.dispatchEvent({type:'change'});assert.equal(x.theme.getResolved(),'light');
});
test('continuous fnOS light/dark events update auto theme',async()=>{
  const x=setup(),h=host();await x.theme.connectHost(h.sdk);
  for(const theme of ['dark','light','dark','light']){h.emit(theme);assert.equal(x.theme.getResolved(),theme);}
});
test('manual override persists while latest host preference is retained',async()=>{
  const x=setup(),h=host();await x.theme.connectHost(h.sdk);x.theme.setPreference('light');h.emit('dark');
  assert.equal(x.theme.getResolved(),'light');assert.equal(x.theme.getSource(),'manual');assert.equal(x.store.get('quarkdrive.theme.preference'),'light');
  x.theme.setPreference('auto');assert.equal(x.theme.getResolved(),'dark');
});
test('standalone browsers use media changes without requesting host APIs',async()=>{
  const x=setup(),h=host('dark',{isStandaloneWeb:true,getPlatformConfig:async()=>{throw Error('must not call');},$on:async()=>{throw Error('must not subscribe');}});
  await x.theme.connectHost(h.sdk);assert.equal(x.theme.getSource(),'browser');x.media.matches=true;x.media.dispatchEvent({type:'change'});assert.equal(x.theme.getResolved(),'dark');
});
test('SDK failure uses browser fallback; stale fnOS storage cannot override it',async()=>{
  const x=setup({darkMedia:true,local:{'fnos-theme-mode':'light'}}),h=host('light',{ready:async()=>{throw Error('old host');}});
  await x.theme.connectHost(h.sdk);assert.equal(x.theme.getResolved(),'dark');assert.equal(x.theme.getSource(),'browser');
  x.media.matches=false;x.media.dispatchEvent({type:'change'});assert.equal(x.theme.getResolved(),'light');
});
test('newer theme event wins over a delayed initial snapshot',async()=>{
  const snapshot=deferred(),x=setup(),h=host('light',{getPlatformConfig:()=>snapshot.promise});
  const pending=x.theme.connectHost(h.sdk);await new Promise(setImmediate);h.emit('dark');snapshot.resolve({theme:'light'});await pending;
  assert.equal(x.theme.getResolved(),'dark');
});
test('pagehide unsubscribes with same callback and removes media listeners',async()=>{
  const x=setup(),h=host();await x.theme.connectHost(h.sdk);const callback=[...h.callbacks][0];x.window.dispatchEvent({type:'pagehide'});
  assert.deepEqual(h.off,[callback]);assert.equal(h.callbacks.size,0);assert.equal(x.media.listenerCount('change'),0);
});
test('subscription finishing after pagehide is immediately removed',async()=>{
  const subscription=deferred(),x=setup();let callback;const off=[];
  const h=host('light',{$on:(event,cb)=>{callback=cb;return subscription.promise;},$off:async(event,cb)=>off.push(cb)});
  const pending=x.theme.connectHost(h.sdk);await new Promise(setImmediate);x.window.dispatchEvent({type:'pagehide'});subscription.resolve();await pending;
  assert.deepEqual(off,[callback]);
});
test('snapshot finishing after pagehide does not change appearance',async()=>{
  const snapshot=deferred(),x=setup(),h=host('dark',{getPlatformConfig:()=>snapshot.promise});const pending=x.theme.connectHost(h.sdk);
  await new Promise(setImmediate);x.window.dispatchEvent({type:'pagehide'});snapshot.resolve({theme:'dark'});await pending;assert.equal(x.theme.getResolved(),'light');
});
test('mobile host reads initial theme without Web-only subscription',async()=>{
  const x=setup(),h=host('dark',{isWeb:false,$on:async()=>{throw Error('mobile must not subscribe');}});await x.theme.connectHost(h.sdk);assert.equal(x.theme.getResolved(),'dark');
});
test('invalid theme falls back until a valid host event arrives',async()=>{
  const x=setup(),h=host('invalid');await x.theme.connectHost(h.sdk);assert.equal(x.theme.getSource(),'browser');h.emit('dark');assert.equal(x.theme.getSource(),'fnos');
});
test('cross-tab preference change updates controls without reading host caches',async()=>{
  const x=setup(),h=host('dark');await x.theme.connectHost(h.sdk);x.store.set('quarkdrive.theme.preference','light');x.window.dispatchEvent({type:'storage',key:'quarkdrive.theme.preference'});assert.equal(x.theme.getResolved(),'light');
});
test('BFCache return reconnects exactly one host subscription',async()=>{
  const x=setup(),h=host();await x.theme.connectHost(h.sdk);x.window.dispatchEvent({type:'pagehide'});x.window.dispatchEvent({type:'pageshow',persisted:true});
  await new Promise(setImmediate);assert.equal(h.callbacks.size,1);h.emit('dark');assert.equal(x.theme.getResolved(),'dark');
});
test('replacing a connection ignores old responses and removes its events',async()=>{
  const snapshot=deferred(),x=setup(),old=host('dark',{getPlatformConfig:()=>snapshot.promise});const pending=x.theme.connectHost(old.sdk);await new Promise(setImmediate);
  const fresh=host('light');await x.theme.connectHost(fresh.sdk);snapshot.resolve({theme:'dark'});await pending;assert.equal(x.theme.getResolved(),'light');assert.equal(old.callbacks.size,0);
});
