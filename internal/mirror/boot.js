// /_/offline/boot.js, served by the HTML Clay host. It removes its own tag before any
// other script runs, so no save ever contains it, then registers HTML Clay's offline
// worker for this origin unless the page runs a worker of its own.
(function () {
  var me = document.currentScript;
  if (me && me.parentNode) me.parentNode.removeChild(me);
  var sw = navigator.serviceWorker;
  if (!sw) return;
  var WORKER = location.origin + '/_/offline/sw.js';
  var origFetch = window.fetch;
  var post = function (path, body) {
    return origFetch.call(window, location.origin + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  };
  var crossOrigin = function (raw) {
    try {
      var u = new URL(raw, location.href);
      return /^https?:$/.test(u.protocol) && u.origin !== location.origin ? u.href : '';
    } catch (e) { return ''; }
  };
  var controlled = !!sw.controller;
  var stopped = false;

  // First open in this browser: until the worker takes control, remember the
  // cross-origin GETs the page makes with fetch() and XHR (Resource Timing cannot tell
  // their method), then put the originals back if nobody replaced them.
  var gets = [];
  var wrapped, wrappedOpen, origOpen = XMLHttpRequest.prototype.open;
  function unwrap() {
    if (wrapped && window.fetch === wrapped) window.fetch = origFetch;
    if (wrappedOpen && XMLHttpRequest.prototype.open === wrappedOpen) XMLHttpRequest.prototype.open = origOpen;
  }
  if (!controlled && origFetch) {
    var note = function (method, raw) {
      var u = crossOrigin(raw);
      if (u && String(method || 'GET').toUpperCase() === 'GET') gets.push({ url: u, kind: 'fetch' });
    };
    wrapped = function (input, init) {
      try {
        var isReq = input && typeof input === 'object' && 'url' in input;
        var method = (init && init.method) || (isReq && input.method) || 'GET';
        var credentials = (init && init.credentials) || (isReq && input.credentials);
        var auth = init && init.headers && new Headers(init.headers).has('authorization');
        if (credentials !== 'include' && !auth) note(method, isReq ? input.url : String(input));
      } catch (e) {}
      return origFetch.apply(this, arguments);
    };
    window.fetch = wrapped;
    wrappedOpen = function (method, raw) {
      if (!this.withCredentials) note(method, raw);
      return origOpen.apply(this, arguments);
    };
    XMLHttpRequest.prototype.open = wrappedOpen;
    sw.addEventListener('controllerchange', unwrap);
    setTimeout(unwrap, 10000);
  }

  // A page that runs its own worker registers it while it loads. HTML Clay watches for
  // that call, waits for load, and steps aside when the page registered a worker or the
  // origin's scope-root worker is not its own.
  var pageRegistered = false;
  var origRegister = sw.register;
  var watchRegister = function (url) {
    try { if (new URL(url, location.href).href !== WORKER) pageRegistered = true; } catch (e) { pageRegistered = true; }
    return origRegister.apply(sw, arguments);
  };
  try { sw.register = watchRegister; } catch (e) {}
  function register() {
    try { if (sw.register === watchRegister) delete sw.register; } catch (e) {}
    if (pageRegistered) { stopped = true; unwrap(); return; }
    sw.getRegistration('/').then(function (reg) {
      var w = reg && (reg.installing || reg.waiting || reg.active);
      if (w && w.scriptURL !== WORKER) { stopped = true; unwrap(); return; }
      return origRegister.call(sw, WORKER, { scope: '/' });
    }).catch(function () {});
  }
  if (document.readyState === 'complete') register(); else addEventListener('load', register);

  if (controlled) return;

  function nominate() {
    if (stopped) return;
    var seen = {};
    var list = [];
    var add = function (url, kind) { if (url && !seen[url]) { seen[url] = 1; list.push({ url: url, kind: kind }); } };
    (performance.getEntriesByType ? performance.getEntriesByType('resource') : []).forEach(function (e) {
      if (['fetch', 'xmlhttprequest', 'beacon', 'video', 'audio', 'track'].indexOf(e.initiatorType) < 0) add(crossOrigin(e.name), e.initiatorType);
    });
    gets.forEach(function (g) { add(g.url, g.kind); });
    if (list.length) post('/_/offline/nominate', list).catch(function () {});
  }

  // Rescue: this browser had no worker yet (a new profile, wiped storage) and the network
  // is down, so cross-origin files failed. Once the worker controls the page, and the host
  // says it holds copies of what failed, reload once, before the person touches the page.
  var failed = [], touched = false, deadline = Date.now() + 10000;
  addEventListener('error', function (e) {
    var t = e.target, u = t && crossOrigin(t.src || t.href || '');
    if (u) failed.push(u);
  }, true);
  ['pointerdown', 'keydown'].forEach(function (n) { addEventListener(n, function () { touched = true; }, true); });
  function maybeRescue() {
    if (stopped || touched || !failed.length || !sw.controller || Date.now() > deadline) return;
    try { if (sessionStorage.getItem('clay-offline-rescued')) return; } catch (e) { return; }
    post('/_/offline/has', failed).then(function (r) { return r.json(); }).then(function (j) {
      if (!j.held || touched || Date.now() > deadline) return;
      sessionStorage.setItem('clay-offline-rescued', '1');
      location.reload();
    }).catch(function () {});
  }
  function afterLoad() { setTimeout(nominate, 300); setTimeout(maybeRescue, 50); }
  if (document.readyState === 'complete') afterLoad(); else addEventListener('load', afterLoad);
  sw.addEventListener('controllerchange', function () { if (document.readyState === 'complete') maybeRescue(); });
})();
