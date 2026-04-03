package model

type Project struct {
	ID      string
	Name    string
	ChatID  int64 // Jika dikosongkan (0), pakai default TelegramChatID
	TopicID int64 // Jika dikosongkan (0), masuk ke General
}

type PipelineResponse []struct {
	ID        int64  `json:"id"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Ref       string `json:"ref"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at"`
}

type CommitResponse struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type AIRequest struct {
	Model       string      `json:"model"`
	Messages    []AIMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens"`
	Temperature float64     `json:"temperature"`
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

type State struct {
	LastID     int64  `json:"id"`
	LastStatus string `json:"status"`
}

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
