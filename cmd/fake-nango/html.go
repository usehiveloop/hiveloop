package main

import "net/http"

// renderAuthHTML mirrors what real Nango ships in
// connections.usehiveloop.com/packages/server/lib/utils/html.ts —
// notifies window.opener via postMessage AND a BroadcastChannel,
// then self-closes. Connect UI listens on both channels; the SDK uses
// the WebSocket but the popup still needs to render and close cleanly.
func renderAuthHTML(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := successHTML
	if errMsg != "" {
		page = errorHTML
	}
	_, _ = w.Write([]byte(page))
}

const successHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Connection successful</title></head>
<body style="font-family:system-ui;background:#0a0a0a;color:#fff;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
<div style="text-align:center">
  <p style="font-size:18px">Successful connection ✅</p>
  <p style="color:#6b7280;font-size:14px">You can close this window.</p>
</div>
<script>
(function () {
  function notify(type, payload) {
    try { if (window.opener) window.opener.postMessage({ type: type, payload: payload }, '*'); } catch (e) {}
    try {
      if (typeof BroadcastChannel !== 'undefined') {
        var ch = new BroadcastChannel('nango-oauth-callback');
        ch.postMessage({ type: type, payload: payload });
        ch.close();
      }
    } catch (e) {}
  }
  notify('AUTHORIZATION_SUCEEDED', null);
  setTimeout(function () { try { window.close(); } catch (e) {} }, 250);
})();
</script>
</body></html>`

const errorHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Connection failed</title></head>
<body style="font-family:system-ui;background:#0a0a0a;color:#fff;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
<div style="text-align:center">
  <p style="font-size:18px">Connection failed ✕</p>
  <p style="color:#6b7280;font-size:14px">You can close this window.</p>
</div>
<script>
(function () {
  function notify(type, payload) {
    try { if (window.opener) window.opener.postMessage({ type: type, payload: payload }, '*'); } catch (e) {}
    try {
      if (typeof BroadcastChannel !== 'undefined') {
        var ch = new BroadcastChannel('nango-oauth-callback');
        ch.postMessage({ type: type, payload: payload });
        ch.close();
      }
    } catch (e) {}
  }
  notify('AUTHORIZATION_FAILED', { message: 'rejected', errorType: 'connection_validation_failed' });
  setTimeout(function () { try { window.close(); } catch (e) {} }, 250);
})();
</script>
</body></html>`
