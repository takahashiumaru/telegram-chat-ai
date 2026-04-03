package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kaguya-telegram/internal/ai"
)

type BotHandler struct {
	Bot       *tgbotapi.BotAPI
	AIService *ai.AIService
}

func NewBotHandler(bot *tgbotapi.BotAPI, aiSvc *ai.AIService) *BotHandler {
	return &BotHandler{
		Bot:       bot,
		AIService: aiSvc,
	}
}

func (h *BotHandler) HandleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	text := strings.TrimSpace(msg.Text)
	lowerText := strings.ToLower(text)
	botMention := "@" + h.Bot.Self.UserName
	mentionAliasesLower := []string{strings.ToLower(botMention), "@thiskaguyabot", "@ThisKaguyaBot"}

	// Handle special commands
	if h.handleSpecialCommands(msg, lowerText, mentionAliasesLower) {
		return
	}

	// Handle AI interaction
	h.handleAIInteraction(msg, text, lowerText, mentionAliasesLower)
}

func (h *BotHandler) handleSpecialCommands(msg *tgbotapi.Message, lowerText string, mentionAliasesLower []string) bool {
	if lowerText == "/ping" || lowerText == "/ping"+mentionAliasesLower[0] {
		h.SendMessage(msg.Chat.ID, "Pong!", msg.MessageID, "")
		return true
	}

	if lowerText == "/id" || lowerText == "/id"+mentionAliasesLower[0] {
		threadID := getThreadID(msg)
		replyMsg := fmt.Sprintf("ℹ️ <b>Info Chat (Umar & Kaguya):</b>\n\n<b>Chat ID:</b> %d\n<b>Thread ID:</b> %d\n<b>Tipe:</b> %s",
			msg.Chat.ID, threadID, msg.Chat.Type)

		if threadID != 0 {
			replyMsg += fmt.Sprintf("\n\n<i>Gunakan ID %d dan Thread %d untuk masuk ke topik ini.</i>", msg.Chat.ID, threadID)
		}

		h.SendMessage(msg.Chat.ID, replyMsg, msg.MessageID, "HTML")
		return true
	}

	return false
}

func (h *BotHandler) handleAIInteraction(msg *tgbotapi.Message, text, lowerText string, mentionAliasesLower []string) {
	var query string
	isAskCommand := false
	hasAlias := containsAliasFold(lowerText, mentionAliasesLower) || mentionInEntities(msg, mentionAliasesLower)

	if payload, ok := parseAskCommand(text, mentionAliasesLower); ok {
		query = payload
		isAskCommand = true
	} else if hasAlias {
		query = strings.TrimSpace(dropMentions(text, mentionAliasesLower...))
		isAskCommand = true
	}

	if isAskCommand {
		if query == "" {
			h.SendMessage(msg.Chat.ID, "Mau nanya apa? Ketik pesannya setelah nama/perintahku ya.", msg.MessageID, "")
			return
		}

		h.Bot.Send(tgbotapi.NewChatAction(msg.Chat.ID, tgbotapi.ChatTyping))
		jawaban := h.AIService.CallAI(query)
		h.SendMessage(msg.Chat.ID, jawaban, msg.MessageID, tgbotapi.ModeMarkdown)
	}
}

func (h *BotHandler) SendMessage(chatID int64, text string, replyTo int, parseMode string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyTo != 0 {
		msg.ReplyToMessageID = replyTo
	}
	if parseMode != "" {
		msg.ParseMode = parseMode
	}
	msg.DisableWebPagePreview = true
	
	// Coba kirim pesan
	if _, err := h.Bot.Send(msg); err != nil {
		// Jika gagal karena masalah ParseMode (karakter spesial AI), kirim ulang sebagai Plain Text
		if strings.Contains(err.Error(), "can't parse entities") || strings.Contains(err.Error(), "parse") {
			log.Printf("[Telegram] Warning: Markdown failed, falling back to plain text. Error: %v", err)
			msg.ParseMode = "" // Hapus parse mode biar jadi teks biasa
			if _, errRetry := h.Bot.Send(msg); errRetry != nil {
				log.Printf("[Telegram] Error sending plain text fallback: %v", errRetry)
			}
		} else {
			log.Printf("[Telegram] Error sending message: %v", err)
		}
	}
}

// Utility functions (internal)

func getThreadID(msg *tgbotapi.Message) int64 {
	// Crude extraction of thread ID from raw JSON as field is missing in v5 library
	var threadID int64
	raw, _ := json.Marshal(msg)
	var data map[string]interface{}
	json.Unmarshal(raw, &data)

	if tID, exists := data["message_thread_id"]; exists {
		if val, ok := tID.(float64); ok {
			threadID = int64(val)
		}
	}
	return threadID
}

func containsAliasFold(text string, aliases []string) bool {
	for _, a := range aliases {
		if strings.Contains(text, a) {
			return true
		}
	}
	return false
}

func mentionInEntities(msg *tgbotapi.Message, aliases []string) bool {
	if msg == nil || len(msg.Entities) == 0 {
		return false
	}
	for _, ent := range msg.Entities {
		if ent.Type != "mention" {
			continue
		}
		if ent.Offset < 0 || ent.Offset+ent.Length > len(msg.Text) {
			continue
		}
		mention := strings.ToLower(msg.Text[ent.Offset : ent.Offset+ent.Length])
		for _, a := range aliases {
			if mention == a {
				return true
			}
		}
	}
	return false
}

func removePrefixFold(s, prefix string) string {
	if len(s) < len(prefix) {
		return s
	}
	if strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):]
	}
	return s
}

func dropMentions(text string, mentions ...string) string {
	fields := strings.Fields(text)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		skip := false
		for _, m := range mentions {
			if strings.EqualFold(f, m) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, f)
		}
	}
	return strings.Join(out, " ")
}

func parseAskCommand(text string, aliases []string) (string, bool) {
	lower := strings.ToLower(text)
	for _, cmd := range []string{"/ask", "ask"} {
		if strings.HasPrefix(lower, cmd) {
			payload := strings.TrimSpace(text[len(cmd):])
			for _, alias := range aliases {
				payload = strings.TrimSpace(removePrefixFold(payload, alias))
			}
			return strings.TrimSpace(payload), true
		}
	}
	return "", false
}
