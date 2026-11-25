'use strict';

// The observer a sync engine is given when the document host watches the folder
// instead of a chokidar watcher, and the follower that feeds it from the host's
// event stream. The engine subscribes to the same stream itself and reruns a
// live session's start on a gap (engine-watcher.js `_onHostGap`); `onGap` is for a
// caller that follows the stream with no engine of its own behind it.

const { EventEmitter } = require('events');

function passiveObserver() {
  const emitter = new EventEmitter();
  const listeners = new Set();
  emitter.subscribe = (listener) => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  };
  emitter.setRemoteApplyCheck = () => {};
  emitter.start = async () => {};
  emitter.stop = async () => {};
  emitter.deliver = (frame) => {
    for (const listener of listeners) listener(frame);
  };
  return emitter;
}

// followHost feeds observer from host's event stream until the returned function
// is called. A `gap` frame (the host lost the cursor, or reconnected without one)
// calls onGap, so the caller can ask for a full comparison.
function followHost(host, observer, { onGap = () => {} } = {}) {
  return host.events(null, (frame) => {
    if (frame.type === 'change') observer.deliver({ event: frame.event, rel: frame.rel });
    else if (frame.type === 'gap') onGap(frame);
  });
}

module.exports = { passiveObserver, followHost };
