const {test} = require('node:test');
const assert = require('node:assert/strict');
const {collectPages} = require('../frontend/assets/file-pages.js');
const files = (start,count) => Array.from({length:count},(_,i)=>({id:String(start+i),name:`文件${start+i}`}));
test('loads all known pages beyond the first 100 files', async()=>{
  const requests=[];
  const result=await collectPages(async page=>{requests.push(page);return {files:files((page-1)*100,page===3?35:100),total:235}});
  assert.equal(result.length,235);assert.deepEqual(requests,[1,2,3]);assert.equal(result.at(-1).id,'234');
});
test('share first page is reused and unknown totals stop on a short page',async()=>{
  const requests=[];
  const result=await collectPages(async page=>{requests.push(page);return {files:files(100,8)}},{firstPage:{files:files(0,100)}});
  assert.equal(result.length,108);assert.deepEqual(requests,[2]);
});
test('known total still loads later data when a batch is smaller than 100',async()=>{
  const result=await collectPages(async page=>({files:files((page-1)*20,20),total:40}));
  assert.equal(result.length,40);
});
test('repeated pages are reported rather than silently truncating or looping',async()=>{
  await assert.rejects(collectPages(async()=>({files:files(0,100),total:200})),/重复返回/);
});
test('unexpected empty page does not claim the directory is complete',async()=>{
  await assert.rejects(collectPages(async page=>({files:page===1?files(0,100):[],total:200})),/未能读取完整列表/);
});
test('overlapping pages deduplicate IDs while preserving the complete list',async()=>{
  const result=await collectPages(async page=>({files:page===1?files(0,100):files(90,20),total:110}));
  assert.equal(result.length,110);
});
test('new navigation discards old results and stops requesting further pages',async()=>{
  let current=true,calls=0;
  const result=await collectPages(async()=>{calls++;current=false;return {files:files(0,100),total:200}},{isCurrent:()=>current});
  assert.equal(result,null);assert.equal(calls,1);
});
test('a later request failure rejects the list instead of returning only the first page',async()=>{
  await assert.rejects(collectPages(async page=>{if(page>1)throw Error('连接失败');return {files:files(0,100),total:150}}),/连接失败/);
});
