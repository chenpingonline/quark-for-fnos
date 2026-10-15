import { TrimApp } from '../vendor/trim-web-app.js';

// Official bridge is bundled locally; no CDN or parent-window probing.
try {
  await window.QuarkDriveTheme.connectHost(new TrimApp());
} catch (_) {
  window.QuarkDriveTheme.apply();
}
