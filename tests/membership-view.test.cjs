const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('frontend/assets/app.js','utf8');
const code=source.slice(source.indexOf('  function renderAccountInfo('),source.indexOf('  async function accountOpen('));
const escapeSource=source.split('\n').find(line=>line.includes('const esc ='));
function render(type,base=''){
  const content={innerHTML:''};
  const renderInfo=vm.runInNewContext(`(()=>{${escapeSource}\n${code};return renderAccountInfo})()`,{$:()=>content,base,bytes:n=>n+' B'});
  renderInfo({nickname:'测试用户',uid:'demo'},type==null?null:{type});return content.innerHTML;
}
test('official member type mapping uses the correct badge including experience tiers',()=>{
  for(const [type,badge] of [['VIP','vip'],['EXP_VIP','vip'],['SUPER_VIP','svip'],['EXP_SVIP','svip'],['Z_VIP','zvip']]){
    const html=render(type,'/app/quarkdrive');assert.ok(html.includes(`src="/app/quarkdrive/assets/membership/${badge}.png"`),type);
    assert.ok(html.indexOf('<h3>')<html.indexOf('<img'),type);assert.ok(html.indexOf('<img')<html.indexOf('</div>'),type);
    assert.ok(!html.includes('88VIP'),type);
  }
});
test('normal, unavailable and unknown tiers never acquire a paid badge',()=>{
  for(const type of ['NORMAL','FUTURE_TIER','constructor','__proto__',null])assert.ok(!render(type).includes('<img'),String(type));
  assert.ok(render('NORMAL').includes('普通用户'));assert.ok(render(null).includes('会员信息暂不可用'));
});
