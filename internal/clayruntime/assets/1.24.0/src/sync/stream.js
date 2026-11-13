const START_TIMEOUT = 5000;
const HIDDEN_DELAY = 5000;
const PING_INTERVAL = 15000;

let workspaceBackground = window.__clayWorkspaceBackground === true;
document.addEventListener('clay:workspace-background', () => { workspaceBackground = true; });
document.addEventListener('clay:workspace-foreground', () => { workspaceBackground = false; });

export const pageHidden = () => document.hidden || workspaceBackground;

export class SyncStream extends EventTarget {
  constructor(url, { shared = false, sharedData = false, documentURL, lane, clientId, startupCheck = false, keepAliveWhenHidden = false, workerFeatures = ['live-data'] } = {}) {
    super();
    this.url = url;
    this.readyState = 0;
    this.onopen = null;
    this.onmessage = null;
    this.onerror = null;
    // Readers keep their own stream on a data-worker host: a reader already lets go
    // of it five seconds after it is hidden, and joining the shared stream would
    // rebuild it under every editor each time a reader tab is shown or hidden.
    this._shared = shared && typeof SharedWorker === 'function' && (!sharedData || lane === 'live');
    this._sharedData = sharedData;
    this._documentURL = documentURL;
    this._lane = lane;
    this._clientId = clientId;
    this._keepAliveWhenHidden = keepAliveWhenHidden;
    this._workerFeatures = workerFeatures;
    this._since = 0;
    this._closed = false;
    this._suspended = false;
    // Two different questions, asked once each at the first subscription and
    // never again: `_repair` is a real reconnect whose replay the host could not
    // retain, and `_startup` is the caller's own check of a page that was served
    // before this stream existed. Only the caller can know whether the response
    // it was served named a version, so the startup check is opt-in and off by
    // default: a stream nobody told about provenance must not manufacture one.
    this._repair = false;
    this._startup = startupCheck;
    this._worker = null;
    this._source = null;
    this._visibility = () => {
      clearTimeout(this._hiddenTimer);
      if (pageHidden()) {
        if (this._worker) this._postWorker(this._worker, 'hidden');
        if (!this._keepHidden()) this._hiddenTimer = setTimeout(() => this._suspend(), HIDDEN_DELAY);
      } else {
        if (this._suspended) this._resume();
        else if (this._worker) {
          this._postWorker(this._worker, 'visible');
        }
      }
    };
    this._pagehide = () => this._suspend(true);
    this._pageshow = () => { if (!pageHidden() || this._keepHidden()) this._resume(); };
    document.addEventListener('visibilitychange', this._visibility);
    document.addEventListener('clay:workspace-background', this._visibility);
    document.addEventListener('clay:workspace-foreground', this._visibility);
    window.addEventListener('pagehide', this._pagehide);
    window.addEventListener('pageshow', this._pageshow);
    this._open();
    if (pageHidden()) this._visibility();
  }

  _emit(type, data) {
    if (this._closed) return;
    const event = data === undefined ? new Event(type) : new MessageEvent(type, { data });
    this.dispatchEvent(event);
    this[`on${type}`]?.(event);
  }

  _postWorker(worker, type, fields = {}) {
    worker.port.postMessage({ v: this._sharedData ? 2 : 1, type, ...fields });
  }

  _open() {
    if (this._closed || this._suspended) return;
    this.readyState = 0;
    if (!this._shared) return this._openDirect();
    try {
      const worker = new SharedWorker(
        new URL(this._sharedData ? '/_/sync/data-worker.js' : '/_/sync/worker.js', window.location.origin).href,
        this._sharedData ? 'clay-sync-data' : 'clay-sync'
      );
      this._worker = worker;
      let lastReply = Date.now();
      let greeted = false;
      worker.onerror = () => {
        if (this._worker !== worker) return;
        this._shared = false;
        this._repair = true;
        this._reopen();
        // Its own connection now: a page already hidden starts letting it go.
        if (pageHidden()) this._visibility();
      };
      this._startTimer = setTimeout(worker.onerror, START_TIMEOUT);
      worker.port.onmessage = ({ data }) => {
        if (this._worker !== worker || data?.v !== (this._sharedData ? 2 : 1)) return;
        if (this._sharedData) {
          // `capacity`: the worker already carries as many documents as one stream
          // may, so this page keeps a stream of its own rather than none.
          if (data.type === 'unsupported' || data.type === 'capacity') {
            worker.onerror();
            return;
          }
          if (data.type === 'gone') {
            this.readyState = 2;
            this._emit('error');
            this.close();
            return;
          }
          if (!greeted) {
            if (data.type !== 'hello') return;
            if (!Array.isArray(data.features) || !this._workerFeatures.every((feature) => data.features.includes(feature))) {
              worker.onerror();
              return;
            }
            greeted = true;
            clearTimeout(this._startTimer);
            lastReply = Date.now();
            return;
          }
          if (data.type === 'hello') return;
        }
        lastReply = Date.now();
        if (!this._sharedData && ['status', 'pong', 'cursor', 'frame', 'gone'].includes(data.type)) clearTimeout(this._startTimer);
        if (data.type === 'status') {
          if (data.state === 'open') {
            if (!this._sharedData || this.readyState !== 1) {
              this.readyState = 1;
              this._emit('open');
            }
          } else if (data.state === 'connecting') {
            this.readyState = 0;
            this._emit('error');
          }
        } else if (data.type === 'cursor') {
          clearTimeout(this._startTimer);
          if (Number.isSafeInteger(data.seq)) this._since = Math.max(this._since, data.seq);
          const resync = data.resync === true || this._repair;
          const startup = this._startup;
          this._repair = false;
          this._startup = false;
          this._emit('cursor', JSON.stringify({ ...data, resync, startup }));
        } else if (data.type === 'frame' && typeof data.data === 'string') {
          this._remember(data.data);
          this._emit('message', data.data);
        } else if (this._sharedData && data.type === 'event' && ['data', 'data-state', 'data-writer', 'presence'].includes(data.event)
          && data.data !== null && typeof data.data === 'object' && !Array.isArray(data.data)) {
          try { this._emit(data.event, JSON.stringify(data.data)); } catch {}
        } else if (data.type === 'gone') {
          this.readyState = 2;
          this._emit('error');
          this.close();
        }
      };
      worker.port.start();
      this._postWorker(worker, pageHidden() ? 'hidden' : 'visible');
      const subscription = { document: this._documentURL, lane: this._lane, since: this._since };
      if (this._sharedData && this._lane === 'live') subscription.clientId = this._clientId;
      this._postWorker(worker, 'subscribe', subscription);
      // A worker is dead when a ping goes unanswered for three intervals, not when
      // three intervals pass between replies: Chrome runs a hidden tab's timers once
      // a minute, so a healthy hidden page sees a minute between its own pings.
      // A tick far later than even that means the page itself was suspended (a
      // sleeping laptop, a frozen tab), and the worker may have swept its port
      // meanwhile without a word: reopen at once rather than wait out the pings.
      let awaitingSince = 0;
      let lastTick = Date.now();
      this._pingTimer = setInterval(() => {
        if (this._worker !== worker) return;
        const now = Date.now();
        const resumed = now - lastTick > PING_INTERVAL * 6;
        lastTick = now;
        if (lastReply >= awaitingSince) awaitingSince = 0;
        if (resumed || (awaitingSince && now - awaitingSince >= PING_INTERVAL * 3)) {
          this._repair = true;
          this._reopen();
          return;
        }
        if (!awaitingSince) awaitingSince = now;
        this._postWorker(worker, 'ping');
      }, PING_INTERVAL);
    } catch {
      this._shared = false;
      this._reopen();
      if (pageHidden()) this._visibility();
    }
  }

  _openDirect() {
    const source = new EventSource(this.url);
    this._source = source;
    let opened = false;
    let hostSentCursor = false;
    source.onopen = () => {
      if (this._source !== source) return;
      // The browser reconnected this stream on its own. A host that keeps no frames
      // never sends a cursor, so nothing says the gap was covered: repair as if it
      // had said `resync`. A host that sends cursors decides that itself.
      if (opened && !hostSentCursor) this._repair = true;
      opened = true;
      this.readyState = 1;
      this._emit('open');
      if (this._repair || this._startup) {
        const resync = this._repair;
        const startup = this._startup;
        this._repair = false;
        this._startup = false;
        this._emit('cursor', JSON.stringify({ resync, startup }));
      }
    };
    source.onerror = () => {
      if (this._source !== source) return;
      this.readyState = source.readyState;
      this._emit('error');
    };
    source.onmessage = event => {
      if (this._source !== source) return;
      this._remember(event.data);
      this._emit('message', event.data);
    };
    // `data`, `data-state` and `data-writer` are clay.live's fast lane (stream-bridge.js).
    for (const type of ['cursor', 'presence', 'data', 'data-state', 'data-writer']) {
      source.addEventListener(type, event => {
        if (this._source !== source) return;
        if (type === 'cursor') hostSentCursor = true;
        this._emit(type, event.data);
      });
    }
  }

  _remember(data) {
    try {
      const { seq } = JSON.parse(data);
      if (Number.isSafeInteger(seq)) this._since = Math.max(this._since, seq);
    } catch {}
  }

  _disconnect() {
    clearTimeout(this._startTimer);
    clearInterval(this._pingTimer);
    if (this._worker) {
      const worker = this._worker;
      this._worker = null;
      worker.onerror = null;
      worker.port.onmessage = null;
      try { this._postWorker(worker, 'unsubscribe'); } catch {}
      worker.port.close();
    }
    if (this._source) {
      this._source.close();
      this._source = null;
    }
  }

  // `keepAliveWhenHidden`: true keeps the stream while hidden (a live-data room);
  // 'shared' keeps it only while it rides the shared worker, so a page that fell
  // back to its own connection still lets go and leaves the browser's six HTTP/1.1
  // connections to the pages on screen.
  _keepHidden() {
    return this._keepAliveWhenHidden === true || (this._keepAliveWhenHidden === 'shared' && this._shared);
  }

  _reopen() {
    const notify = (this._sharedData || this._keepAliveWhenHidden === true) && this.readyState === 1;
    this.readyState = 0;
    if (notify) this._emit('error');
    this._disconnect();
    this._open();
  }

  _suspend(pageExit = false) {
    clearTimeout(this._hiddenTimer);
    if (this._closed || this._suspended) return;
    this._suspended = true;
    this._repair = true;
    this.readyState = 0;
    if (pageExit && this._keepAliveWhenHidden === true) this._emit('error');
    this._disconnect();
  }

  _resume() {
    clearTimeout(this._hiddenTimer);
    if (this._closed || !this._suspended) return;
    this._suspended = false;
    this._open();
  }

  close() {
    this._closed = true;
    this.readyState = 2;
    clearTimeout(this._hiddenTimer);
    this._disconnect();
    document.removeEventListener('visibilitychange', this._visibility);
    document.removeEventListener('clay:workspace-background', this._visibility);
    document.removeEventListener('clay:workspace-foreground', this._visibility);
    window.removeEventListener('pagehide', this._pagehide);
    window.removeEventListener('pageshow', this._pageshow);
  }
}
