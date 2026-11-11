package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Optional AI assistant, served by the owner's Hermes agent through its
// OpenAI-compatible API server. Off unless HERMES_API_URL is set.
// Hermes owns the model, tools (finance skills that read this API) and long-term
// memory; Kantong only keeps the visible chat history and replays the last
// chatHistory messages each turn.

const chatHistory = 40

var hermesClient = &http.Client{Timeout: 5 * time.Minute} // agent turns with tool calls can be slow

type chatMsg struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Model     string `json:"model,omitempty"`
	CreatedAt string `json:"created_at"`
}

// chatModels parses CHAT_MODELS ("provider:model,provider:model"); the first is the default.
func chatModels(s string) []string {
	var out []string
	for _, m := range strings.Split(s, ",") {
		if m = strings.TrimSpace(m); strings.Contains(m, ":") {
			out = append(out, m)
		}
	}
	return out
}

func (a *app) chatOn(w http.ResponseWriter) bool {
	if a.cfg.hermesURL == "" {
		writeErr(w, http.StatusNotFound, "chat is off: set HERMES_API_URL to enable it")
		return false
	}
	return true
}

func (a *app) getChat(w http.ResponseWriter, r *http.Request) {
	if a.cfg.hermesURL == "" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	msgs, err := a.chatMessages(chatHistory * 2)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "messages": msgs, "models": chatModels(a.cfg.chatModels)})
}

func (a *app) clearChat(w http.ResponseWriter, r *http.Request) {
	if !a.chatOn(w) {
		return
	}
	if _, err := a.db.Exec(`DELETE FROM chat_messages`); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) chatMessages(limit int) ([]chatMsg, error) {
	rows, err := a.db.Query(`SELECT id, role, text, model, created_at FROM (SELECT * FROM chat_messages ORDER BY id DESC LIMIT ?) ORDER BY id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []chatMsg{}
	for rows.Next() {
		var m chatMsg
		if err := rows.Scan(&m.ID, &m.Role, &m.Text, &m.Model, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *app) postChat(w http.ResponseWriter, r *http.Request) {
	if !a.chatOn(w) {
		return
	}
	var in struct {
		Text  string `json:"text"`
		Model string `json:"model"` // one of CHAT_MODELS; empty = first one, or Hermes' own default
	}
	if !decode(w, r, &in) {
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" || len(in.Text) > 4000 {
		writeErr(w, http.StatusBadRequest, "text must be 1-4000 characters")
		return
	}
	models := chatModels(a.cfg.chatModels)
	if in.Model == "" && len(models) > 0 {
		in.Model = models[0]
	}
	if in.Model != "" && !slices.Contains(models, in.Model) {
		writeErr(w, http.StatusBadRequest, "unknown model")
		return
	}

	history, err := a.chatMessages(chatHistory)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reply, err := a.askHermes(r, history, in.Text, in.Model)
	if err != nil {
		slog.Error("chat", "err", err)
		writeErr(w, http.StatusBadGateway, "Hermes gagal menjawab: "+err.Error())
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	var out [2]chatMsg
	for i, m := range [][3]string{{"user", in.Text, ""}, {"assistant", reply, in.Model}} {
		if err := tx.QueryRow(`INSERT INTO chat_messages (role, text, model) VALUES (?, ?, ?) RETURNING id, role, text, model, created_at`, m[0], m[1], m[2]).
			Scan(&out[i].ID, &out[i].Role, &out[i].Text, &out[i].Model, &out[i].CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

func (a *app) askHermes(r *http.Request, history []chatMsg, text, model string) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	now := time.Now().In(wib)
	msgs := []msg{{"system", fmt.Sprintf(`Pesan ini dari chat Asisten di web app Kantong (money manager milik pemilikmu), bukan dari Telegram.
Bertindak sebagai asisten keuangan pribadinya: jawab soal pengeluaran, budget, tagihan rutin, dan aset dengan data asli dari API Kantong (pakai skill finance), jangan menebak angka.
Jawab dalam bahasa Indonesia santai, singkat, langsung ke inti. Nominal ditulis seperti "Rp 1.250.000".
Jangan mengubah, menambah, atau menghapus data kecuali pemilik memintanya dengan jelas di pesan ini.
Sekarang: %s WIB.`, now.Format("2006-01-02 15:04 (Monday)"))}}
	for _, m := range history {
		msgs = append(msgs, msg{m.Role, m.Text})
	}
	msgs = append(msgs, msg{"user", text})

	body := map[string]any{"model": "hermes-agent", "messages": msgs}
	if provider, name, ok := strings.Cut(model, ":"); ok {
		body["provider"], body["model"] = provider, name
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(a.cfg.hermesURL, "/")+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.hermesKey)
	req.Header.Set("X-Hermes-Session-Id", "kantong-web") // one ongoing conversation, like the visible history

	res, err := hermesClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	resBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %.200s", res.StatusCode, resBody)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(resBody, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("unexpected response: %.200s", resBody)
	}
	reply := strings.TrimSpace(out.Choices[0].Message.Content)
	if reply == "" {
		return "", fmt.Errorf("empty reply")
	}
	return reply, nil
}
