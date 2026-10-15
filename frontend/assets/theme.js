/* Official fnOS theme takes precedence over browser appearance in auto mode. */
(function () {
  'use strict';
  const root = document.documentElement;
  const PREFERENCE_KEY = 'quarkdrive.theme.preference';
  const MODES = ['auto', 'light', 'dark'];
  const media = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-color-scheme: dark)') : null;
  let preference = readPreference();
  let hostTheme = null;
  let stopHost = null;
  let lastSDK = null;
  let suspended = false;

  function readPreference() {
    try { const value = window.localStorage.getItem(PREFERENCE_KEY); return MODES.includes(value) ? value : 'auto'; }
    catch (_) { return 'auto'; }
  }
  function normalize(value) { return value === 'light' || value === 'dark' ? value : null; }
  function apply() {
    const resolved = preference !== 'auto' ? preference : hostTheme || (media?.matches ? 'dark' : 'light');
    root.setAttribute('data-theme', resolved);
    root.setAttribute('data-theme-preference', preference);
    root.setAttribute('data-theme-source', preference !== 'auto' ? 'manual' : hostTheme ? 'fnos' : 'browser');
    root.style.colorScheme = resolved;
    const select = document.getElementById('setting-theme');
    if (select && select.value !== preference) select.value = preference;
    window.dispatchEvent(new Event('quarkdrive:theme'));
  }
  function setPreference(next) {
    if (!MODES.includes(next)) return;
    preference = next;
    try { window.localStorage.setItem(PREFERENCE_KEY, next); } catch (_) {}
    apply();
  }
  async function connectHost(sdk) {
    if (stopHost) stopHost();
    lastSDK = sdk;
    hostTheme = null;
    let closed = false, registered = false, revision = 0;
    const receiveTheme = value => {
      const theme = normalize(value);
      if (closed || suspended || !theme) return;
      revision++;
      hostTheme = theme;
      apply();
    };
    const unsubscribe = () => {
      if (!registered) return;
      registered = false;
      try { Promise.resolve(sdk.$off('os/theme', receiveTheme)).catch(() => {}); } catch (_) {}
    };
    stopHost = () => { closed = true; unsubscribe(); };
    apply();
    try {
      if (sdk.ready) await sdk.ready();
      if (closed || suspended || sdk.isStandaloneWeb) return;
      // Subscribe first so a newer theme event cannot be overwritten by an older snapshot.
      let subscription = Promise.resolve();
      if (sdk.isWeb === true) {
        try {
          subscription = Promise.resolve(sdk.$on('os/theme', receiveTheme)).then(() => {
            registered = true;
            if (closed || suspended) unsubscribe();
          }).catch(() => {});
        } catch (_) { /* A host may provide the snapshot without event support. */ }
      }
      const beforeSnapshot = revision;
      const config = await sdk.getPlatformConfig();
      if (!closed && !suspended && revision === beforeSnapshot) receiveTheme(config?.theme);
      await subscription;
    } catch (_) {
      // Standalone browsers, unavailable SDK and older hosts use the media fallback.
      if (!closed && !suspended) apply();
    }
  }
  function storageChanged(event) {
    if (event.key === null || event.key === PREFERENCE_KEY) { preference = readPreference(); apply(); }
  }
  function listen() {
    window.addEventListener('storage', storageChanged);
    if (media?.addEventListener) media.addEventListener('change', apply);
    else if (media?.addListener) media.addListener(apply);
  }
  function suspend() {
    suspended = true;
    if (stopHost) stopHost();
    window.removeEventListener('storage', storageChanged);
    if (media?.removeEventListener) media.removeEventListener('change', apply);
    else if (media?.removeListener) media.removeListener(apply);
  }
  window.addEventListener('pagehide', suspend);
  window.addEventListener('pageshow', event => {
    if (!event.persisted || !suspended) return;
    suspended = false; preference = readPreference(); listen(); apply();
    if (lastSDK) void connectHost(lastSDK);
  });
  window.QuarkDriveTheme = { apply, setPreference, connectHost, getPreference: () => preference,
    getResolved: () => root.getAttribute('data-theme'), getSource: () => root.getAttribute('data-theme-source') };
  listen();
  apply(); // Synchronous fallback before CSS; the host snapshot then overrides it.
})();
