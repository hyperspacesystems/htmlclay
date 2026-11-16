// Command folderlock-helper holds and tests the per-folder serve locks for a
// program that cannot take them itself. Hyperclay Local, which is Node, starts
// one and talks to it over stdin and stdout, one JSON object per line. Every
// lock it holds is released when its stdin closes, so a parent that quits or
// crashes never leaves a folder locked.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"

	"github.com/panphora/htmlclay/internal/folderlock"
)

type request struct {
	ReqID   string `json:"reqId"`
	Op      string `json:"op"`
	ID      string `json:"id"`
	LockDir string `json:"lockDir"`
	Folder  string `json:"folder"`
}

type response struct {
	ReqID string `json:"reqId"`
	OK    bool   `json:"ok"`
	Held  bool   `json:"held,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	held := make(map[string]*folderlock.Lock)
	defer func() {
		for _, l := range held {
			_ = l.Release()
		}
	}()

	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad-json"})
			continue
		}
		switch req.Op {
		case "acquire":
			if existing, ok := held[req.ID]; ok {
				_ = existing.Release()
				delete(held, req.ID)
			}
			l, err := folderlock.Acquire(req.LockDir, req.Folder)
			if err != nil {
				if errors.Is(err, folderlock.ErrHeld) {
					_ = enc.Encode(response{ReqID: req.ReqID, OK: false, Held: true})
				} else {
					_ = enc.Encode(response{ReqID: req.ReqID, OK: false, Error: err.Error()})
				}
				continue
			}
			held[req.ID] = l
			_ = enc.Encode(response{ReqID: req.ReqID, OK: true})
		case "release":
			if existing, ok := held[req.ID]; ok {
				_ = existing.Release()
				delete(held, req.ID)
			}
			_ = enc.Encode(response{ReqID: req.ReqID, OK: true})
		default:
			_ = enc.Encode(response{ReqID: req.ReqID, OK: false, Error: "unknown-op"})
		}
	}
}
