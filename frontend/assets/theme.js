/* QuarkDrive appearance bridge: fnOS host preference -> CSS variables.
 * Works without network/CDN and keeps a per-browser override.
 * In a same-origin fnOS iframe, parent storage and DOM are consulted.
 */
(function () {
  'use strict';
  const root = document.documentElement;
  const PREFERENCE_KEY = 'quarkdrive.theme.preference';
  const FNOS_KEYS = ['fnos-theme-mode', 'theme-mode'];
  const MODES = ['auto', 'light', 'dark'];
  const media = typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)') : null;
  let preference = readPreference();
  let observer = null;

  function safeGet(store, key) {
    try { return store && store.getItem(key); } catch (_) { return null; }
  }
  function readPreference() {
    try {
      const value = safeGet(window.localStorage, PREFERENCE_KEY);
      return MODES.includes(value) ? value : 'auto';
    } catch (_) { return 'auto'; }
  }
  function normalize(value) {
    if (typeof value !== 'string') return null;
    const v = value.trim().toLowerCase().replace(/^['"]|['"]$/g, '');
    if (['dark', 'night', 'darkmode'].includes(v)) return 'dark';
    if (['light', 'day', 'lightmode'].includes(v)) return 'light';
    return null;
  }
  function configTheme(value) {
    if (typeof value !== 'string' || value.length > 100000) return null;
    try {
      const parsed = JSON.parse(value);
      if (!parsed || typeof parsed !== 'object') return null;
      const theme = parsed.theme ?? parsed.themeMode ?? parsed.theme_mode ??
        parsed.appearance ?? parsed.mode ?? parsed.setting?.theme ??
        parsed.settings?.theme ?? parsed.desktop?.theme;
      return normalize(theme);
    } catch (_) { return null; }
  }
  function hostWindow() {
    try {
      if (window.parent && window.parent !== window && window.parent.document) {
        // Accessing parent.document throws for a cross-origin host.
        void window.parent.document.documentElement;
        return window.parent;
      }
    } catch (_) { /* cross-origin: local/system fallback */ }
    return null;
  }
  function themeFromDOM(win) {
    if (!win || !win.document) return null;
    for (const element of [win.document.documentElement, win.document.body]) {
      if (!element) continue;
      for (const attr of ['theme-mode', 'data-theme', 'data-theme-mode']) {
        const result = normalize(element.getAttribute(attr));
        if (result) return result;
      }
      if (element.classList && element.classList.contains('dark')) return 'dark';
    }
    return null;
  }
  function themeFromStorage(win) {
    if (!win) return null;
    let storage;
    try { storage = win.localStorage; } catch (_) { return null; }
    for (const key of FNOS_KEYS) {
      const theme = normalize(safeGet(storage, key));
      if (theme) return theme;
    }
    // Some fnOS versions save desktop preferences per user.
    try {
      for (let i = 0; i < storage.length; i++) {
        const key = storage.key(i);
        if (/^DesktopConfig-\d+$/.test(key)) {
          const theme = configTheme(safeGet(storage, key));
          if (theme) return theme;
        }
      }
    } catch (_) { /* unavailable storage */ }
    return null;
  }
  function systemTheme() {
    const host = hostWindow();
    return themeFromStorage(host) || themeFromDOM(host) ||
      themeFromStorage(window) || (media && media.matches ? 'dark' : 'light');
  }
  function apply() {
    const resolved = preference === 'auto' ? systemTheme() : preference;
    root.setAttribute('data-theme', resolved);
    root.setAttribute('data-theme-preference', preference);
    root.style.colorScheme = resolved;
    const select = document.getElementById('setting-theme');
    if (select && select.value !== preference) select.value = preference;
  }
  function setPreference(next) {
    if (!MODES.includes(next)) return;
    preference = next;
    try { window.localStorage.setItem(PREFERENCE_KEY, next); } catch (_) {}
    apply();
  }
  function attachHostObserver() {
    const host = hostWindow();
    if (!host || typeof MutationObserver !== 'function') return;
    try {
      observer = new MutationObserver(apply);
      if (host.document.documentElement) observer.observe(host.document.documentElement,
        { attributes: true, attributeFilter: ['theme-mode', 'data-theme', 'data-theme-mode', 'class'] });
      if (host.document.body) observer.observe(host.document.body,
        { attributes: true, attributeFilter: ['theme-mode', 'data-theme', 'data-theme-mode', 'class'] });
    } catch (_) { if (observer) observer.disconnect(); }
  }
  window.addEventListener('storage', (event) => {
    if (event.key === PREFERENCE_KEY) preference = readPreference();
    if (event.key === null || event.key === PREFERENCE_KEY ||
        FNOS_KEYS.includes(event.key) || /^DesktopConfig-\d+$/.test(event.key || '')) apply();
  });
  if (media) {
    if (media.addEventListener) media.addEventListener('change', apply);
    else if (media.addListener) media.addListener(apply);
  }
  window.addEventListener('focus', apply);
  document.addEventListener('visibilitychange', () => { if (!document.hidden) apply(); });
  document.addEventListener('DOMContentLoaded', () => { attachHostObserver(); apply(); });
  window.QuarkDriveTheme = { apply, setPreference, getPreference: () => preference, getResolved: () => root.getAttribute('data-theme') };
  apply(); // Before CSS loads: avoids flashing the wrong theme.
})();
