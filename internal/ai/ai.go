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
			{Role: "system", Content: "Kamu adalah Kaguya, asisten programmer AI dari Indonesia. Gaya bicaramu santai, asik, friendly, dan menggunakan bahasa pergaulan sehari-hari (seperti menggunakan kata 'gue', 'lu', 'bro', 'mantap', 'oke'). Kamu sangat ahli dalam coding, debugging, dan IT. Jangan pernah menjawab dengan bahasa yang terlalu kaku atau formal seperti robot. Ketika menjelaskan kode, selalu berikan contoh praktis dan relevan."},
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Origin", "https://kaguya-ai.vercel.app")
	req.Header.Set("Referer", "https://kaguya-ai.vercel.app/")
	req.Header.Set("sec-ch-ua", `"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "cross-site")

	log.Printf("[AI] Sending browser-mimic request to %s", fullEndpoint)
	
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
