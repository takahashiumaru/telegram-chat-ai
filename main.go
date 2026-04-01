package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	{ID: "59214445", Name: "visit-flow-presence"},
	{ID: "46744032", Name: "visit-flow-api-gateway"},
	{ID: "76428296", Name: "visit-flow-payroll"},
	{ID: "46743800", Name: "visit-flow-survey-location-go"},
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
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	Ref       string `json:"ref"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at"`
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

var bot *tgbotapi.BotAPI

type State struct {
	LastID     int64  `json:"id"`
	LastStatus string `json:"status"`
}

func saveState(states map[string]State) {
	data, _ := json.Marshal(states)
	_ = os.WriteFile("state.json", data, 0644)
}

func loadState() map[string]State {
	data, err := os.ReadFile("state.json")
	if err != nil {
		return make(map[string]State)
	}
	var states map[string]State
	json.Unmarshal(data, &states)
	return states
}

var lastReported = make(map[string]State)

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

		isNewId := (latestPipeline.ID != last.LastID)
		isNewStatus := (latestPipeline.Status != last.LastStatus)

		// Parse waktu pipeline (GitLab pakai ISO 8601)
		updatedAt, err := time.Parse(time.RFC3339, latestPipeline.UpdatedAt)
		if err != nil {
			updatedAt = time.Now()
		}

		// Hitung selisih waktu
		timeSinceUpdate := time.Since(updatedAt)

		// Hanya kirim jika baru (ID/Status berubah) DAN masih dalam jendela 30 menit
		if (isNewId || isNewStatus) &&
			(latestPipeline.Status == "success" || latestPipeline.Status == "failed" || latestPipeline.Status == "running" || latestPipeline.Status == "pending") &&
			(timeSinceUpdate.Minutes() <= 30) {

			var statusIcon string
			switch latestPipeline.Status {
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

			msgText := fmt.Sprintf("%s <b>Deploy Update!</b>\n\n<b>Repo:</b> %s\n<b>Pipeline ID:</b> %d\n<b>Branch:</b> %s\n<b>Status:</b> %s\n\n🔗 <a href=\"%s\">Buka GitLab Pipeline</a>",
				statusIcon, project.Name, latestPipeline.ID, latestPipeline.Ref, latestPipeline.Status, latestPipeline.WebURL)

			log.Printf("[%s] Mengirim notifikasi %s (ID: %d, Time: %v ago)...", project.Name, latestPipeline.Status, latestPipeline.ID, timeSinceUpdate)

			var targetChatID int64
			fmt.Sscanf(TelegramChatID, "%d", &targetChatID)

			notification := tgbotapi.NewMessage(targetChatID, msgText)
			notification.ParseMode = "HTML"
			bot.Send(notification)

			lastReported[project.ID] = State{LastID: latestPipeline.ID, LastStatus: latestPipeline.Status}
			saveState(lastReported)
		} else if isNewId || isNewStatus {
			// Update state jika status berubah tapi tidak dikirim (karena sudah lewat 30 menit)
			lastReported[project.ID] = State{LastID: latestPipeline.ID, LastStatus: latestPipeline.Status}
			saveState(lastReported)
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

	// Load state dari file
	lastReported = loadState()

	// PENTING: Hapus webhook agar metode Long Polling (di Render/Lokal) bisa menerima pesan.
	bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})

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

		// Bot sekarang akan merespons di grup mana pun atau di chat pribadi langsung.
		// Jika Anda ingin membatasi lagi nanti, silakan masukkan filter grup di sini.

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
