package gitlab

import (
	"log"
	"time"
	"kaguya-telegram/internal/config"
	"kaguya-telegram/internal/model"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type PipelineMonitor struct {
	Client *Client
	Bot    *tgbotapi.BotAPI
}

func NewPipelineMonitor(client *Client, bot *tgbotapi.BotAPI) *PipelineMonitor {
	return &PipelineMonitor{
		Client: client,
		Bot:    bot,
	}
}

func (m *PipelineMonitor) Start() {
	log.Println("GitLab Pipeline Monitor started...")
	for {
		for _, p := range config.Projects {
			m.monitorProject(p)
		}
		time.Sleep(30 * time.Second)
	}
}

func (m *PipelineMonitor) monitorProject(project model.Project) {
	msgText, shouldNotify := m.Client.CheckPipeline(project)
	if !shouldNotify {
		return
	}

	targetChatID := config.GetTelegramChatID()
	if project.ChatID != 0 {
		targetChatID = project.ChatID
	}

	notification := tgbotapi.NewMessage(targetChatID, msgText)
	notification.ParseMode = "HTML"
	if project.TopicID != 0 {
		notification.BaseChat.ReplyToMessageID = int(project.TopicID)
	}

	log.Printf("[%s] Sending pipeline notification...", project.Name)
	if _, err := m.Bot.Send(notification); err != nil {
		log.Printf("[%s] Error sending notification: %v", project.Name, err)
	}
}
