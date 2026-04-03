package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	req, err := http.NewRequest("POST", s.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "Maaf, ada masalah saat menyiapkan permintaan ke AI."
	}

	req.Header.Set("Authorization", "Bearer "+s.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KaguyaTelegramBot/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Maaf, aku tidak bisa menghubungi server AI saat ini."
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
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
