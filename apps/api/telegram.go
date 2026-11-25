package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	tgTokenRe = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{20,}$`)
	tgChatRe  = regexp.MustCompile(`^-?\d{1,20}$`)
)

// tgCipher encrypts bot tokens at rest. The key is derived from SESSION_SECRET: rotating that
// secret makes stored tokens unreadable, and each account has to connect its bot again.
func (a *app) tgCipher() cipher.AEAD {
	m := hmac.New(sha256.New, []byte(a.cfg.sessionSecret))
	m.Write([]byte("kantong telegram bot token"))
	block, _ := aes.NewCipher(m.Sum(nil))
	gcm, _ := cipher.NewGCMWithRandomNonce(block)
	return gcm
}

func (a *app) sealToken(tok string) string {
	return b64.EncodeToString(a.tgCipher().Seal(nil, nil, []byte(tok), nil))
}

func (a *app) openToken(enc string) (string, error) {
	raw, err := b64.DecodeString(enc)
	if err != nil {
		return "", err
	}
	plain, err := a.tgCipher().Open(nil, nil, raw, nil)
	return string(plain), err
}

// telegramFor returns the bot token and chat an account's notifications go to, or "" when it has none.
// An account with only a chat uses the server's .env bot (shared, so nobody needs its token); the first
// account with nothing set falls back to the .env bot and chat.
func (a *app) telegramFor(uid int64) (token, chat string) {
	var enc string
	a.db.QueryRow(`SELECT tg_token, tg_chat FROM users WHERE id = ?`, uid).Scan(&enc, &chat)
	if enc != "" {
		tok, err := a.openToken(enc)
		if err != nil {
			slog.Warn("telegram: stored bot token unreadable (SESSION_SECRET changed?); reconnect it", "user", uid)
			return "", ""
		}
		return tok, chat
	}
	if chat != "" {
		return a.cfg.tgToken, chat
	}
	if uid == a.ownerID {
		return a.cfg.tgToken, a.cfg.tgChat
	}
	return "", ""
}

func (a *app) sendTelegram(token, chat, text string) error {
	c := http.Client{Timeout: 10 * time.Second}
	res, err := c.PostForm(a.cfg.tgAPI+"/bot"+token+"/sendMessage", url.Values{"chat_id": {chat}, "text": {text}})
	if err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), token, "***")) // the URL holds the token
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram HTTP %d", res.StatusCode)
	}
	return nil
}

type me struct {
	Username string `json:"username"`
	Telegram struct {
		Connected bool   `json:"connected"`
		ChatID    string `json:"chat_id"`
		FromEnv   bool   `json:"from_env"`   // the first account's bot and chat from .env, not set in the app
		OwnBot    bool   `json:"own_bot"`    // the account's own bot token, else the server's bot
		ServerBot bool   `json:"server_bot"` // the server has a .env bot, so a chat id alone is enough
	} `json:"telegram"`
}

// getMe returns the signed-in account. The bot token itself never leaves the server.
func (a *app) getMe(w http.ResponseWriter, r *http.Request) {
	var m me
	var enc string
	uid := userID(r)
	if err := a.db.QueryRow(`SELECT username, tg_token, tg_chat FROM users WHERE id = ?`, uid).Scan(&m.Username, &enc, &m.Telegram.ChatID); err != nil {
		a.fail(w, err)
		return
	}
	m.Telegram.OwnBot, m.Telegram.ServerBot = enc != "", a.cfg.tgToken != ""
	if !m.Telegram.ServerBot && !m.Telegram.OwnBot {
		m.Telegram.ChatID = "" // a chat for the server bot that .env no longer has
	}
	if m.Telegram.ChatID == "" && uid == a.ownerID && a.cfg.tgToken != "" && a.cfg.tgChat != "" {
		m.Telegram.ChatID, m.Telegram.FromEnv = a.cfg.tgChat, true
	}
	m.Telegram.Connected = m.Telegram.ChatID != ""
	writeJSON(w, http.StatusOK, m)
}

// setTelegram connects the account's Telegram: {"bot_token": "123:ABC…", "chat_id": "42"}, or only a chat_id
// to use the server's bot. It sends a test message first, so a wrong token or a chat that never pressed
// Start is reported instead of stored. Both empty disconnects (the first account then falls back to .env).
func (a *app) setTelegram(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	tok, chat := strings.TrimSpace(in.BotToken), strings.TrimSpace(in.ChatID)
	enc := ""
	if tok != "" || chat != "" {
		send := tok
		switch {
		case tok == "" && a.cfg.tgToken == "":
			writeErr(w, http.StatusBadRequest, "this server has no shared bot: paste your own bot token from @BotFather")
			return
		case tok == "":
			send = a.cfg.tgToken
		case !tgTokenRe.MatchString(tok):
			writeErr(w, http.StatusBadRequest, "bot token looks wrong: copy it from @BotFather (123456:ABC…)")
			return
		case !tgChatRe.MatchString(chat):
			writeErr(w, http.StatusBadRequest, "chat id must be a number")
			return
		}
		if err := a.sendTelegram(send, chat, "Kantong terhubung. Notifikasi transaksi dari email akan dikirim ke sini."); err != nil {
			writeErr(w, http.StatusBadRequest, "test message failed ("+err.Error()+"): check the chat id (and token), and press Start in the chat with the bot")
			return
		}
		if tok != "" {
			enc = a.sealToken(tok)
		}
	}
	if _, err := a.db.Exec(`UPDATE users SET tg_token = ?, tg_chat = ? WHERE id = ?`, enc, chat, userID(r)); err != nil {
		a.fail(w, err)
		return
	}
	a.getMe(w, r)
}
