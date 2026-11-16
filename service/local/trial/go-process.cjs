'use strict';

let connection = null;

function setConnection(next) {
  if (!next || typeof next.url !== 'string' || typeof next.secret !== 'string') {
    throw new TypeError('setConnection needs the control url and the secret');
  }
  connection = {
    url: next.url.replace(/\/+$/, ''),
    secret: next.secret,
    epoch: next.epoch ?? null,
    peerEpoch: next.peerEpoch ?? null,
    roots: { ...(next.roots ?? {}) },
  };
  return connection;
}

function getConnection() {
  if (!connection) throw new Error('the Go document host is not connected');
  return connection;
}

function clearConnection() {
  connection = null;
}

module.exports = { setConnection, getConnection, clearConnection };
