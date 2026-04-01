package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- KONFIGURASI ---
const (
	TelegramBotToken = "6293769087:AAHgRTAHAJj3yG6KC6dex3iNlYgUQjAJr0o"
	TelegramChatID   = "-1003859941008" // ID untuk notifikasi GitLab
	TelegramTopicID  = 1419            // ID Topic (Thread) untuk notifikasi
	GroqAPIKey       = "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"
)

// --- STRUCT DATA ---

// Struct untuk GitLab Webhook (Pipeline Event)
type GitlabWebhookPayload struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		ID        int64  `json:"id"`
		Status    string `json:"status"`
		Ref       string `json:"ref"`
		CreatedAt string `json:"created_at"`
	} `json:"object_attributes"`
	Project struct {
		Name   string `json:"name"`
		WebURL string `json:"web_url"`
	} `json:"project"`
	Commit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Title   string `json:"title"`
	} `json:"commit"`
	User struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"user"`
}

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

var (
	bot     *tgbotapi.BotAPI
	initErr error
	
	// Cache dan Mutex untuk deduplikasi (Bertahan selama proses warm start)
	sentCache = make(map[string]string) 
	cacheMu   sync.Mutex
)

var allowedGroupIDs = map[int64]struct{}{
	-1003521971868: {},
	-1003859941008: {},
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

// Handler is the main entry point for Vercel Serverless Function
func Handler(w http.ResponseWriter, r *http.Request) {
	// Inisialisasi bot global sekali
	if bot == nil {
		bot, initErr = tgbotapi.NewBotAPI(TelegramBotToken)
		if initErr != nil {
			http.Error(w, "init bot error", http.StatusInternalServerError)
			return
		}
	}

	// Setup Helper: Kunjungi URL https://domain-mu.vercel.app/?setup=true untuk mendaftarkan bot.
	if r.URL.Query().Get("setup") == "true" {
		webhookURL := "https://" + r.Host + "/"
		config, _ := tgbotapi.NewWebhook(webhookURL)
		_, err := bot.Request(config)
		if err != nil {
			fmt.Fprintf(w, "Setup Error: %v", err)
			return
		}
		fmt.Fprintf(w, "Setup Success! Webhook set to: %s", webhookURL)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// 1. CEK APAKAH INI DARI GITLAB WEBHOOK?
	// GitLab biasanya mengirim Header "X-Gitlab-Event"
	if r.Header.Get("X-Gitlab-Event") != "" {
		handleGitlabWebhook(body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "gitlab ok")
		return
	}

	// 2. CEK APAKAH INI DARI TELEGRAM UPDATE?
	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		w.WriteHeader(http.StatusOK) // Abaikan jika bukan format valid
		return
	}

	if update.Message != nil {
		handleTelegramMessage(update, body)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

// --- LOGIKA GITLAB WEBHOOK (Untuk Vercel) ---
func handleGitlabWebhook(body []byte) {
	var payload GitlabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("GitLab Parse Error: %v", err)
		return
	}

	// Hanya proses jika ini adalah Pipeline Event
	if payload.ObjectKind != "pipeline" {
		return
	}

	status := payload.ObjectAttributes.Status

	// Parsing waktu pembuatan pipeline (Kombinasi format GitLab)
	createdAt, err := time.Parse("2006-01-02 15:04:05 MST", payload.ObjectAttributes.CreatedAt)
	if err != nil {
		createdAt, err = time.Parse(time.RFC3339, payload.ObjectAttributes.CreatedAt)
		if err != nil {
			createdAt = time.Now()
		}
	}

	// Cek apakah event ini "baru" (di bawah 30 menit)
	if time.Since(createdAt).Minutes() > 30 {
		log.Printf("GitLab Webhook diabaikan karena pipeline sudah lama (ID: %d, CreatedAt: %s)", payload.ObjectAttributes.ID, payload.ObjectAttributes.CreatedAt)
		return
	}

	// DEDUPLIKASI: Cek cache global
	cacheMu.Lock()
	lastStatus, ok := sentCache[fmt.Sprintf("%d", payload.ObjectAttributes.ID)]
	cacheMu.Unlock()
	
	if ok && lastStatus == status {
		log.Printf("Deduplikasi: ID %d dengan status %s sudah dikirim sebelumnya.", payload.ObjectAttributes.ID, status)
		return
	}

	// Filter status yg dikirim (running, success, failed, pending)
	if status == "success" || status == "failed" || status == "running" || status == "pending" {
		var statusIcon string
		switch status {
		case "success":
			statusIcon = "✅"
		case "failed":
			statusIcon = "❌"
		case "running":
			statusIcon = "⏳"
		case "pending":
			statusIcon = "🕘"
		default:
			statusIcon = "🔄"
		}

		// URL Pipeline di Webhook berbeda tempat (Project.WebURL + /-/pipelines/ID)
		pipelineURL := fmt.Sprintf("%s/-/pipelines/%d", payload.Project.Name, payload.ObjectAttributes.ID)
		// Jika Project.WebURL ada, gunakan itu
		if payload.Project.WebURL != "" {
			pipelineURL = fmt.Sprintf("%s/-/pipelines/%d", payload.Project.WebURL, payload.ObjectAttributes.ID)
		}

		// Ambil commit message (Title adalah baris pertama commit)
		commitMsg := payload.Commit.Title
		if commitMsg == "" {
			commitMsg = payload.Commit.Message
		}

		msgText := fmt.Sprintf("%s <b>Deploy Update!</b>\n\n<b>Repo:</b> %s\n<b>Branch:</b> %s\n<b>Status:</b> %s\n<b>Commit:</b> %s\n<b>By:</b> %s\n\n🔗 <a href=\"%s\">Buka GitLab Pipeline</a>",
			statusIcon, payload.Project.Name, payload.ObjectAttributes.Ref, status, commitMsg, payload.User.Name, pipelineURL)

		var targetChatID int64
		fmt.Sscanf(TelegramChatID, "%d", &targetChatID)
		
		msg := tgbotapi.NewMessage(targetChatID, msgText)
		msg.ParseMode = "HTML"
		
		// Set Topic ID jika ada
		if TelegramTopicID != 0 {
			msg.BaseChat.ReplyToMessageID = TelegramTopicID
		}
		
		bot.Send(msg)

		// Simpan ke cache untuk deduplikasi
		cacheMu.Lock()
		sentCache[fmt.Sprintf("%d", payload.ObjectAttributes.ID)] = status
		cacheMu.Unlock()
	}
}

// --- LOGIKA TELEGRAM AI ---
func handleTelegramMessage(update tgbotapi.Update, rawBody []byte) {
	msg := update.Message
	// Filter Grup
	if !msg.Chat.IsGroup() && !msg.Chat.IsSuperGroup() {
		return
	}
	if len(allowedGroupIDs) > 0 {
		if _, ok := allowedGroupIDs[msg.Chat.ID]; !ok {
			return
		}
	}

	text := strings.TrimSpace(msg.Text)
	lowerText := strings.ToLower(text)
	botMention := "@" + bot.Self.UserName
	mentionAliasesLower := []string{strings.ToLower(botMention), "@thiskaguyabot", "@ThisKaguyaBot"}

	// --- 1. HANDLE PERINTAH KHUSUS (/ping, /id) ---
	if lowerText == "/ping" || lowerText == "/ping"+mentionAliasesLower[0] {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Pong!")
		reply.ReplyToMessageID = msg.MessageID
		bot.Send(reply)
		return
	}

	if lowerText == "/id" || lowerText == "/id"+mentionAliasesLower[0] {
		// Ambil Thread ID LANGSUNG dari raw JSON
		var threadID int64 = 0
		var rawMap map[string]interface{}
		json.Unmarshal(rawBody, &rawMap)

		if msgMap, ok := rawMap["message"].(map[string]interface{}); ok {
			if tID, exists := msgMap["message_thread_id"]; exists {
				if val, ok := tID.(float64); ok {
					threadID = int64(val)
				}
			}
		}

		replyMsg := fmt.Sprintf("ℹ️ <b>Info Chat:</b>\n\n<b>Chat ID:</b> %d\n<b>Thread ID:</b> %d\n<b>Tipe:</b> %s",
			msg.Chat.ID, threadID, msg.Chat.Type)
		
		if threadID != 0 {
			replyMsg += fmt.Sprintf("\n\n<i>Gunakan ID %d dan Thread %d</i>", msg.Chat.ID, threadID)
		}

		reply := tgbotapi.NewMessage(msg.Chat.ID, replyMsg)
		reply.ReplyToMessageID = msg.MessageID
		reply.ParseMode = "HTML"
		bot.Send(reply)
		return
	}

	// --- 2. HANDLE AI BOT ---
	var query string
	isAskCommand := false
	hasAlias := containsAliasFold(lowerText, mentionAliasesLower) || mentionInEntities(msg, mentionAliasesLower) || (msg.ReplyToMessage != nil && msg.ReplyToMessage.From.ID == bot.Self.ID)

	// Deteksi /ask atau mention
	if payload, ok := parseAskCommand(text, mentionAliasesLower); ok {
		query = payload
		isAskCommand = true
	} else if hasAlias {
		query = strings.TrimSpace(dropMentions(text, mentionAliasesLower...))
		isAskCommand = true
	}

	if isAskCommand && query != "" {
		jawaban := callAI(query)
		reply := tgbotapi.NewMessage(msg.Chat.ID, jawaban)
		reply.ReplyToMessageID = msg.MessageID
		reply.ParseMode = tgbotapi.ModeMarkdown
		reply.DisableWebPagePreview = true
		bot.Send(reply)
	}
}

// --- UTILS (SAMA SEPERTI MAIN.GO) ---

func containsAliasFold(text string, aliases []string) bool {
	for _, a := range aliases {
		if strings.Contains(text, a) {
			return true
		}
	}
	return false
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

func removePrefixFold(s, prefix string) string {
	if len(s) < len(prefix) {
		return s
	}
	if strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):]
	}
	return s
}

func callAI(query string) string {
	reqBody := AIRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []AIMessage{
			{Role: "system", Content: "Kamu adalah asisten AI bernama Kaguya yang cerdas dan ramah di Telegram. Berikan jawaban yang membantu dan sopan."},
			{Role: "user", Content: query},
		},
	}
	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+GroqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Maaf, server AI sedang sibuk."
	}
	defer resp.Body.Close()

	var aiResp AIResponse
	json.NewDecoder(resp.Body).Decode(&aiResp)
	if len(aiResp.Choices) > 0 {
		return aiResp.Choices[0].Message.Content
	}
	return "Maaf, terjadi kesalahan."
}
