package recalc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

var errPipeClosed = errors.New("cdp pipe closed")

type cdpMsg struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *cdpError) Error() string {
	return fmt.Sprintf("cdp error %d: %s", e.Code, e.Message)
}

type cdpConn struct {
	w       io.WriteCloser
	wmu     sync.Mutex
	nextID  atomic.Int64
	pending sync.Map // int64 -> chan cdpMsg
	onEvent func(cdpMsg)
	closed  chan struct{}
	once    sync.Once
}

func newCDPConn(r io.ReadCloser, w io.WriteCloser, onEvent func(cdpMsg)) *cdpConn {
	c := &cdpConn{
		w:       w,
		onEvent: onEvent,
		closed:  make(chan struct{}),
	}
	go c.readLoop(r)
	return c
}

func (c *cdpConn) markClosed() {
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *cdpConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *cdpConn) readLoop(r io.ReadCloser) {
	defer func() {
		_ = r.Close()
		c.markClosed()
	}()
	br := bufio.NewReaderSize(r, 4*1024*1024)
	for {
		raw, err := br.ReadBytes(0)
		if len(raw) > 0 && raw[len(raw)-1] == 0 {
			raw = raw[:len(raw)-1]
		}
		if len(raw) > 0 {
			var m cdpMsg
			if json.Unmarshal(raw, &m) == nil {
				if m.ID != 0 {
					if ch, ok := c.pending.LoadAndDelete(m.ID); ok {
						ch.(chan cdpMsg) <- m
					}
				} else if c.onEvent != nil {
					c.onEvent(m)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (c *cdpConn) writeFrame(sessionID, method string, params any) (int64, error) {
	id := c.nextID.Add(1)
	req := map[string]any{"id": id, "method": method}
	if sessionID != "" {
		req["sessionId"] = sessionID
	}
	if params != nil {
		req["params"] = params
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	payload = append(payload, 0)
	c.wmu.Lock()
	_, err = c.w.Write(payload)
	c.wmu.Unlock()
	return id, err
}

func (c *cdpConn) sendAsync(sessionID, method string, params any) {
	if c.isClosed() {
		return
	}
	_, _ = c.writeFrame(sessionID, method, params)
}

func (c *cdpConn) call(ctx context.Context, sessionID, method string, params any) (json.RawMessage, error) {
	if c.isClosed() {
		return nil, errPipeClosed
	}
	id := c.nextID.Add(1)
	ch := make(chan cdpMsg, 1)
	c.pending.Store(id, ch)

	req := map[string]any{"id": id, "method": method}
	if sessionID != "" {
		req["sessionId"] = sessionID
	}
	if params != nil {
		req["params"] = params
	}
	payload, err := json.Marshal(req)
	if err != nil {
		c.pending.Delete(id)
		return nil, err
	}
	payload = append(payload, 0)

	c.wmu.Lock()
	_, err = c.w.Write(payload)
	c.wmu.Unlock()
	if err != nil {
		c.pending.Delete(id)
		c.markClosed()
		return nil, err
	}

	select {
	case res := <-ch:
		if res.Error != nil {
			return nil, fmt.Errorf("cdp %s: %w", method, res.Error)
		}
		return res.Result, nil
	case <-c.closed:
		c.pending.Delete(id)
		return nil, errPipeClosed
	case <-ctx.Done():
		c.pending.Delete(id)
		return nil, ctx.Err()
	}
}
