package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *hub { return &hub{clients: map[chan []byte]struct{}{}} }

func (h *hub) subscribe() chan []byte {
	c := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *hub) unsubscribe(c chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c)
	}
}

func (h *hub) publish(event string, v any) {
	data, _ := json.Marshal(v)
	msg := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, data)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- msg:
		default: // client too slow: drop it; it reconnects and refetches
			delete(h.clients, c)
			close(c)
		}
	}
}

func (a *app) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	c := a.hub.subscribe()
	defer a.hub.unsubscribe(c)

	w.Write([]byte(": connected\n\n"))
	if err := rc.Flush(); err != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		var msg []byte
		select {
		case <-r.Context().Done():
			return
		case m, ok := <-c:
			if !ok {
				return
			}
			msg = m
		case <-ping.C:
			msg = []byte(": ping\n\n")
		}
		if _, err := w.Write(msg); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
