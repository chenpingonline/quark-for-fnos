const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../frontend/assets/theme.js'), 'utf8');
function storage(initial = {}) {
  const map = new Map(Object.entries(initial));
  return { getItem: k => map.has(k) ? map.get(k) : null,
    setItem: (k, v) => map.set(k, String(v)),
    key: i => [...map.keys()][i] || null, get length() { return map.size; } };
}
function element(attributes = {}) {
  const attrs = new Map(Object.entries(attributes));
  return { getAttribute: k => attrs.get(k) || null,
    setAttribute: (k, v) => attrs.set(k, v),
    classList: { contains: () => false }, style: {} };
}
function setup({ fnos, attr, local = {}, darkMedia = false, blockedParent = false } = {}) {
  const events = {}, mediaEvents = {};
  const host = { document: { documentElement: element(attr || {}), body: element() },
    localStorage: storage(fnos ? {'fnos-theme-mode': fnos} : {}) };
  const localStorage = storage(local);
  const self = {document: {}, localStorage,
    addEventListener(type, f) { events[type] = f; },
    matchMedia() { return { matches: darkMedia, addEventListener(type, f) { mediaEvents[type] = f; } }; }};
  self.parent = blockedParent ? { get document() { throw Error('cross origin'); } } : host;
  const root = element();
  const docEvents = {};
  const doc = { documentElement: root, body: element(), hidden: false,
    getElementById() { return null; },
    addEventListener(type, f) { docEvents[type] = f; } };
  const context = { window: self, document: doc, MutationObserver: class { observe() {} disconnect() {} } };
  vm.runInNewContext(code, context, {filename:'theme.js'});
  return { theme: self.QuarkDriveTheme, host, localStorage, events, mediaEvents, docEvents, root };
}
test('fnOS dark mode wins over OS media preference', () => {
  const x = setup({fnos:'dark'});
  assert.equal(x.theme.getResolved(), 'dark');
  assert.equal(x.root.style.colorScheme, 'dark');
});
test('fnOS switching light/dark updates automatically', () => {
  const x = setup({fnos:'light'});
  assert.equal(x.theme.getResolved(), 'light');
  x.host.localStorage.setItem('fnos-theme-mode', 'dark');
  x.events.storage({key:'fnos-theme-mode'});
  assert.equal(x.theme.getResolved(), 'dark');
});
test('manual preference persists and overrides host until reset', () => {
  const x = setup({fnos:'dark'});
  x.theme.setPreference('light');
  assert.equal(x.theme.getResolved(), 'light');
  assert.equal(x.localStorage.getItem('quarkdrive.theme.preference'), 'light');
  x.theme.setPreference('auto');
  assert.equal(x.theme.getResolved(), 'dark');
});
test('when storage lacks theme, fnOS DOM attribute is consulted', () => {
  const x = setup({attr:{'theme-mode':'dark'}});
  assert.equal(x.theme.getResolved(), 'dark');
});
test('when fnOS is inaccessible, OS dark preference is used', () => {
  const x = setup({blockedParent:true, darkMedia:true});
  assert.equal(x.theme.getResolved(), 'dark');
});
test('invalid saved preferences fall back to auto', () => {
  const x = setup({fnos:'dark', local:{'quarkdrive.theme.preference':'purple'}});
  assert.equal(x.theme.getPreference(), 'auto');
  assert.equal(x.theme.getResolved(), 'dark');
});
test('desktop per-user config supports theme', () => {
  const x = setup();
  x.host.localStorage.setItem('DesktopConfig-1000', JSON.stringify({themeMode:'dark'}));
  x.events.storage({key:'DesktopConfig-1000'});
  assert.equal(x.theme.getResolved(), 'dark');
});
