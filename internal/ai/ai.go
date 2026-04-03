package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"kaguya-telegram/internal/config"
	"kaguya-telegram/internal/model"
)

type AIService struct {
	ApiKey    string
	Endpoint  string
	ModelName string
}

func NewAIService() *AIService {
	return &AIService{
		ApiKey:    config.GroqAPIKey,
		Endpoint:  config.GroqAPIEndpoint,
		ModelName: "llama-3.3-70b-versatile",
	}
}

func (s *AIService) CallAI(query string) string {
	reqBody := model.AIRequest{
		Model: s.ModelName,
		Messages: []model.AIMessage{
			{Role: "system", Content: "Kamu adalah asisten AI bernama Kaguya yang cerdas dan ramah di Telegram. Berikan jawaban yang membantu dan sopan."},
			{Role: "user", Content: query},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "Maaf, terjadi kesalahan teknis (marshal)."
	}

	// Otomatis tambahkan path jika tidak ada (mencegah 404)
	fullEndpoint := s.Endpoint
	if !strings.HasSuffix(fullEndpoint, "/openai/v1/chat/completions") && !strings.Contains(fullEndpoint, "api.groq.com") {
		// Jika ini adalah proxy (bukan Groq asli) dan tidak punya path, tambahkan.
		fullEndpoint = strings.TrimSuffix(fullEndpoint, "/") + "/openai/v1/chat/completions"
	}

	req, err := http.NewRequest("POST", fullEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "Maaf, ada masalah saat menyiapkan permintaan ke AI."
	}

	req.Header.Set("Authorization", "Bearer "+s.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KaguyaTelegramBot/2.0")
	req.Header.Set("Accept", "application/json")

	log.Printf("[AI] Sending request to %s", s.Endpoint)
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[AI] Connection Error: %v", err)
		return "⚠️ [ERROR] Bot tidak bisa menghubungi server AI (Koneksi ditolak/Timeout)."
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[AI] %d HTTP Error: %s", resp.StatusCode, string(body))
		
		if resp.StatusCode == 403 {
			return "🚫 [403] IP VPS Anda diblokir oleh Groq/Cloudflare. Solusi: Gunakan Cloudflare Worker Proxy."
		}
		if resp.StatusCode == 404 || resp.StatusCode == 1042 {
			return fmt.Sprintf("⚠️ [%d] Endpoint bermasalah. Pastikan URL Cloudflare Worker Anda benar.", resp.StatusCode)
		}
		return fmt.Sprintf("AI error (%d): %s", resp.StatusCode, truncate(string(body), 400))
	}

	var aiResp model.AIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil || len(aiResp.Choices) == 0 || aiResp.Choices[0].Message.Content == "" {
		return "Maaf, terjadi kesalahan saat memproses jawaban AI."
	}

	return aiResp.Choices[0].Message.Content
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
