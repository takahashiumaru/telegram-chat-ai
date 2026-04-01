package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Struktur data untuk OpenAI-Compatible API (Groq)
type AIRequest struct {
	Model    string      `json:"model"`
	Messages []AIMessage `json:"messages"`
}

type AIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AIResponse struct {
	Choices []struct {
		Message AIMessage `json:"message"`
	} `json:"choices"`
}

// containsAliasFold checks if any alias appears (case-insensitive) in text.
func containsAliasFold(text string, aliases []string) bool {
	for _, a := range aliases {
		if strings.Contains(text, a) {
			return true
		}
	}
	return false
}

// mentionInEntities checks message entities for mentions matching aliases.
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

// removePrefixFold trims prefix in a case-insensitive manner if present.
func removePrefixFold(s, prefix string) string {
	if len(s) < len(prefix) {
		return s
	}
	if strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):]
	}
	return s
}

// dropMentions removes any word that equals a mention (case-insensitive).
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

// parseAskCommand returns payload if message starts with /ask or ask (case-insensitive).
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Fungsi untuk memanggil AI (OpenAI API Compatible)
func callAI(query string) string {
	apiKey := "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"
	if apiKey == "" {
		return "Maaf, GROQ_API_KEY belum di-set."
	}

	endpoint := "https://api.groq.com/openai/v1/chat/completions" // Endpoint Groq
	modelName := "llama-3.3-70b-versatile"                        // Model Groq

	reqBody := AIRequest{
		Model: modelName,
		Messages: []AIMessage{
			{Role: "system", Content: "Kamu adalah asisten AI bernama Kaguya yang cerdas dan ramah di Telegram. Berikan jawaban yang membantu dan sopan."},
			{Role: "user", Content: query},
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "Maaf, ada masalah saat menyiapkan permintaan ke AI."
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "Maaf, aku tidak bisa menghubungi server AI saat ini."
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("AI error (%d): %s", resp.StatusCode, truncate(string(body), 400))
	}

	var aiResp AIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil || len(aiResp.Choices) == 0 || aiResp.Choices[0].Message.Content == "" {
		return fmt.Sprintf("AI parse error: %v | body: %s", err, truncate(string(body), 400))
	}

	return aiResp.Choices[0].Message.Content
}

// Only these group IDs can use the bot; leave empty to allow all groups.
// Replace the example with your actual group IDs (negative IDs for supergroups).
var allowedGroupIDs = map[int64]struct{}{
	// Isi dengan ID grup yang diizinkan. Kosongkan untuk izinkan semua.
	-1003521971868: {},
}

func main() {
	token := "6293769087:AAHgRTAHAJj3yG6KC6dex3iNlYgUQjAJr0o"
	if token == "" {
		log.Fatal("set TELEGRAM_BOT_TOKEN dulu")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}
	log.Printf("Bot aktif sebagai @%s", bot.Self.UserName)

	// Pastikan tidak ada webhook aktif yang membuat getUpdates conflict
	if _, err := bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true}); err != nil {
		log.Fatalf("hapus webhook: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("Menerima pesan dari tipe chat: %s, ID: %d, teks: %s", update.Message.Chat.Type, update.Message.Chat.ID, update.Message.Text)

		// Hanya tanggapi grup/supergroup
		if !update.Message.Chat.IsGroup() && !update.Message.Chat.IsSuperGroup() {
			reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Maaf, aku saat ini hanya ngobrol di grup/supergroup saja :)")
			bot.Send(reply)
			continue
		}

		// Batasi hanya grup yang diizinkan jika daftar tidak kosong
		if len(allowedGroupIDs) > 0 {
			if _, ok := allowedGroupIDs[update.Message.Chat.ID]; !ok {
				continue
			}
		}

		msg := update.Message
		text := strings.TrimSpace(msg.Text)
		lowerText := strings.ToLower(text)

		botMention := "@" + bot.Self.UserName
		mentionAliasesLower := []string{strings.ToLower(botMention), "@thiskaguyabot"}
		botNameLower := mentionAliasesLower[0]

		var query string
		isAskCommand := false

		hasAlias := containsAliasFold(lowerText, mentionAliasesLower) || mentionInEntities(msg, mentionAliasesLower)

		// /ask atau ask (opsional diikuti @bot) lalu pertanyaan
		if payload, ok := parseAskCommand(text, mentionAliasesLower); ok {
			query = payload
			isAskCommand = true
		} else if hasAlias {
			// Mention bot langsung
			query = strings.TrimSpace(dropMentions(text, mentionAliasesLower...))
			isAskCommand = true
		}

		if isAskCommand {
			if query == "" {
				reply := tgbotapi.NewMessage(msg.Chat.ID, "Mau nanya apa? Ketik pesannya setelah nama/perintahku ya.")
				reply.ReplyToMessageID = msg.MessageID
				bot.Send(reply)
				continue
			}

			// Kasih efek "Typing..." (sedang mengetik) sambil menunggu balasan AI
			typingAction := tgbotapi.NewChatAction(msg.Chat.ID, tgbotapi.ChatTyping)
			bot.Send(typingAction)

			// Memanggil AI
			jawaban := callAI(query)

			reply := tgbotapi.NewMessage(msg.Chat.ID, jawaban)
			reply.ReplyToMessageID = msg.MessageID
			reply.ParseMode = tgbotapi.ModeMarkdown
			reply.DisableWebPagePreview = true
			bot.Send(reply)
			continue
		}

		switch {
		case lowerText == "/ping" || lowerText == "/ping"+botNameLower:
			reply := tgbotapi.NewMessage(msg.Chat.ID, "Pong!")
			reply.ReplyToMessageID = msg.MessageID
			bot.Send(reply)
		}
	}
}
