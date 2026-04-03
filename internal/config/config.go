package config

import (
	"kaguya-telegram/internal/model"
	"os"
	"strconv"
)

var (
	GitlabToken      = getEnv("GITLAB_TOKEN", "")
	GitlabAPIURL     = "https://gitlab.com/api/v4/projects/%s/pipelines?per_page=1"
	TelegramBotToken = getEnv("TELEGRAM_BOT_TOKEN", "")
	TelegramChatID   = getEnv("TELEGRAM_CHAT_ID", "-1003859941008") 
	AIKey            = getEnv("AI_API_KEY", "")
	AIEndpoint       = getEnv("AI_API_ENDPOINT", "https://models.inference.ai.azure.com/chat/completions")
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
