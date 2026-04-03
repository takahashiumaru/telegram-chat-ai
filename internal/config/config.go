package config

import (
	"os"
	"strconv"
	"kaguya-telegram/internal/model"
)

var (
	GitlabToken      = getEnv("GITLAB_TOKEN", "glpat-jo57tfUU5LsuIEFc_G4DGmM6MQpvOjEKdTo3NHpxbg8.01.170yd21nc")
	GitlabAPIURL     = "https://gitlab.com/api/v4/projects/%s/pipelines?per_page=1"
	TelegramBotToken = getEnv("TELEGRAM_BOT_TOKEN", "6293769087:AAHgRTAHAJj3yG6KC6dex3iNlYgUQjAJr0o")
	TelegramChatID   = getEnv("TELEGRAM_CHAT_ID", "-1003859941008")
	GroqAPIKey       = getEnv("GROQ_API_KEY", "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")
	// PAKE URL PROXY ASLI LU BRO BIAR TEMBUS!
	GroqAPIEndpoint  = getEnv("GROQ_API_ENDPOINT", "https://groq-proxy.umarmarufmutaqin.workers.dev/")
	StateFilePath    = "state.json"
)

var Projects = []model.Project{
	{ID: "39997609", Name: "mf-micro-service-discount-proposal", TopicID: 1419},
	{ID: "45966760", Name: "visit-flow-go", TopicID: 1419},
	{ID: "59214445", Name: "visit-flow-presence", TopicID: 1419},
	{ID: "46744032", Name: "visit-flow-api-gateway", TopicID: 1419},
	{ID: "76428296", Name: "visit-flow-payroll", TopicID: 1419},
	{ID: "46743800", Name: "visit-flow-survey-location-go", TopicID: 1419},
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func GetTelegramChatID() int64 {
	id, _ := strconv.ParseInt(TelegramChatID, 10, 64)
	return id
}
