package main

import (
	"log"

	"github.com/joho/godotenv"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kaguya-telegram/internal/ai"
	"kaguya-telegram/internal/config"
	"kaguya-telegram/internal/gitlab"
	"kaguya-telegram/internal/state"
	"kaguya-telegram/internal/telegram"
)

func main() {
	// Load .env file for local development
	_ = godotenv.Load()
	// 1. Initialize core services
	bot, err := tgbotapi.NewBotAPI(config.GetTelegramBotToken())
	if err != nil {
		log.Fatalf("Fatal: failed to initialize bot: %v", err)
	}
	log.Printf("Bot Kaguya active as @%s", bot.Self.UserName)

	// Clean up webhooks for long polling
	if _, err := bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true}); err != nil {
		log.Printf("Warning: failed to delete webhook: %v", err)
	}

	stateMgr := state.NewStateManager()
	aiService := ai.NewAIService()
	gitlabClient := gitlab.NewClient(stateMgr)
	botHandler := telegram.NewBotHandler(bot, aiService)

	// 2. Start GitLab monitor in background
	monitor := gitlab.NewPipelineMonitor(gitlabClient, bot)
	go monitor.Start()

	// 3. Start Telegram listener loop
	log.Println("Starting Telegram updates listener...")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		botHandler.HandleUpdate(update)
	}
}
