package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- KONFIGURASI GITLAB ---
type Project struct {
	ID   string
	Name string
}

var Projects = []Project{
	{ID: "39997609", Name: "mf-micro-service-discount-proposal"},
	{ID: "45966760", Name: "visit-flow-go"},
}

const (
	GitlabToken  = "glpat-jo57tfUU5LsuIEFc_G4DGmM6MQpvOjEKdTo3NHpxbg8.01.170yd21nc"
	GitlabAPIURL = "https://gitlab.com/api/v4/projects/%s/pipelines?per_page=1"
)

// --- KONFIGURASI TELEGRAM ---
const (
	TelegramBotToken = "6293769087:AAHgRTAHAJj3yG6KC6dex3iNlYgUQjAJr0o"
	TelegramChatID   = "1631339759" // ID Chat untuk Notifikasi GitLab
)

// --- STRUCT DATA ---

type PipelineResponse []struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	WebURL string `json:"web_url"`
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

// Map untuk melacak status terakhir per project ID (GitLab)
var lastReported = make(map[string]struct {
	ID     int64
	Status string
})

var bot *tgbotapi.BotAPI

// --- FUNGSI AI & UTILS ---

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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func callAI(query string) string {
	apiKey := "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"
	endpoint := "https://api.groq.com/openai/v1/chat/completions"
	modelName := "llama-3.3-70b-versatile"

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

	client := &http.Client{Timeout: 10 * time.Second}
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
		return "Maaf, terjadi kesalahan saat memproses jawaban AI."
	}

	return aiResp.Choices[0].Message.Content
}

// --- FUNGSI MONITOR GITLAB ---

func checkGitlabPipeline(project Project) {
	url := fmt.Sprintf(GitlabAPIURL, project.ID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("PRIVATE-TOKEN", GitlabToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[%s] Error HTTP: %v", project.Name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	var pipelines PipelineResponse
	if err := json.NewDecoder(resp.Body).Decode(&pipelines); err != nil {
		return
	}

	if len(pipelines) > 0 {
		latestPipeline := pipelines[0]
		last := lastReported[project.ID]

		isNewId := (latestPipeline.ID != last.ID)
		isNewStatus := (latestPipeline.Status != last.Status)

		if (isNewId || isNewStatus) && (latestPipeline.Status == "success" || latestPipeline.Status == "failed" || latestPipeline.Status == "running") {
			var statusIcon string
			switch latestPipeline.Status {
			case "success":
				statusIcon = "✅"
			case "failed":
				statusIcon = "❌"
			case "running":
				statusIcon = "⏳"
			default:
				statusIcon = "🔄"
			}

			msgText := fmt.Sprintf("%s <b>Deploy Update!</b>\n\n<b>Repo:</b> %s\n<b>Pipeline ID:</b> %d\n<b>Branch:</b> %s\n<b>Status:</b> %s\n\n🔗 <a href=\"%s\">Buka GitLab Pipeline</a>",
				statusIcon, project.Name, latestPipeline.ID, latestPipeline.Ref, latestPipeline.Status, latestPipeline.WebURL)

			log.Printf("[%s] Mengirim notifikasi %s...", project.Name, latestPipeline.Status)
			
			// Kirim ke Telegram
			chatID, _ := time.ParseDuration("0s") // dummy to use ChatID correctly
			_ = chatID
			
			// Konversi string ChatID ke int64
			var targetChatID int64
			fmt.Sscanf(TelegramChatID, "%d", &targetChatID)
			
			notification := tgbotapi.NewMessage(targetChatID, msgText)
			notification.ParseMode = "HTML"
			bot.Send(notification)

			lastReported[project.ID] = struct {
				ID     int64
				Status string
			}{ID: latestPipeline.ID, Status: latestPipeline.Status}
		} else if isNewStatus {
			lastReported[project.ID] = struct {
				ID     int64
				Status string
			}{ID: latestPipeline.ID, Status: latestPipeline.Status}
		}
	}
}

func monitorGitlabLoop() {
	for {
		for _, p := range Projects {
			checkGitlabPipeline(p)
		}
		time.Sleep(30 * time.Second)
	}
}

// --- MAIN ENTRY POINT ---

var allowedGroupIDs = map[int64]struct{}{
	-1003521971868: {},
	-1003859941008: {},
}

func main() {
	var err error
	bot, err = tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}
	log.Printf("Bot Kaguya aktif sebagai @%s", bot.Self.UserName)

	// Hapus webhook agar getUpdates lancar (HANYA AKTIFKAN JIKA INGIN LOKAL SAJA TANPA VERCEL)
	// bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})

	// Jalankan Monitor GitLab di Background (Goroutine)
	go monitorGitlabLoop()

	// Jalankan Listener Chat Telegram (AI Bot)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		msg := update.Message
		
		// Filter Grup
		if !msg.Chat.IsGroup() && !msg.Chat.IsSuperGroup() {
			continue
		}
		if len(allowedGroupIDs) > 0 {
			if _, ok := allowedGroupIDs[msg.Chat.ID]; !ok {
				continue
			}
		}

		text := strings.TrimSpace(msg.Text)
		lowerText := strings.ToLower(text)
		botMention := "@" + bot.Self.UserName
		mentionAliasesLower := []string{strings.ToLower(botMention), "@thiskaguyabot", "@ThisKaguyaBot"}

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
				reply := tgbotapi.NewMessage(msg.Chat.ID, "Mau nanya apa? Ketik pesannya setelah nama/perintahku ya.")
				reply.ReplyToMessageID = msg.MessageID
				bot.Send(reply)
				continue
			}

			bot.Send(tgbotapi.NewChatAction(msg.Chat.ID, tgbotapi.ChatTyping))
			jawaban := callAI(query)

			reply := tgbotapi.NewMessage(msg.Chat.ID, jawaban)
			reply.ReplyToMessageID = msg.MessageID
			reply.ParseMode = tgbotapi.ModeMarkdown
			reply.DisableWebPagePreview = true
			bot.Send(reply)
			continue
		}

		if lowerText == "/ping" || lowerText == "/ping"+mentionAliasesLower[0] {
			reply := tgbotapi.NewMessage(msg.Chat.ID, "Pong!")
			reply.ReplyToMessageID = msg.MessageID
			bot.Send(reply)
		}
	}
}
