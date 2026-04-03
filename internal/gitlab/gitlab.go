package gitlab

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"kaguya-telegram/internal/config"
	"kaguya-telegram/internal/model"
	"kaguya-telegram/internal/state"
)

type Client struct {
	Token string
	SM    *state.StateManager
}

func NewClient(sm *state.StateManager) *Client {
	return &Client{
		Token: config.GitlabToken,
		SM:    sm,
	}
}

func (c *Client) FetchPipelines(projectID string) (model.PipelineResponse, error) {
	url := fmt.Sprintf(config.GitlabAPIURL, projectID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("PRIVATE-TOKEN", c.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API error: %d", resp.StatusCode)
	}

	var pipelines model.PipelineResponse
	if err := json.NewDecoder(resp.Body).Decode(&pipelines); err != nil {
		return nil, err
	}

	return pipelines, nil
}

func (c *Client) FetchCommitDetail(projectID, sha string) (string, error) {
	if sha == "" {
		return "No commit message", nil
	}
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/repository/commits/%s", projectID, sha)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("PRIVATE-TOKEN", c.Token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var commitDetail model.CommitResponse
	if err := json.NewDecoder(resp.Body).Decode(&commitDetail); err != nil {
		return "", err
	}

	if commitDetail.Title != "" {
		return commitDetail.Title, nil
	}
	return truncate(commitDetail.Message, 100), nil
}

func (c *Client) FetchPipelineUser(projectID string, pipelineID int64) (string, error) {
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/pipelines/%d", projectID, pipelineID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("PRIVATE-TOKEN", c.Token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var pipeDetail struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pipeDetail); err != nil {
		return "", err
	}

	if pipeDetail.User.Name != "" {
		return pipeDetail.User.Name, nil
	}
	return "Unknown", nil
}

func (c *Client) CheckPipeline(project model.Project) (string, bool) {
	pipelines, err := c.FetchPipelines(project.ID)
	if err != nil {
		log.Printf("[%s] Fetch Error: %v", project.Name, err)
		return "", false
	}

	if len(pipelines) == 0 {
		return "", false
	}

	latest := pipelines[0]
	last, ok := c.SM.Get(project.ID)

	isNewID := !ok || latest.ID != last.LastID
	isNewStatus := !ok || latest.Status != last.LastStatus

	updatedAt, parseErr := time.Parse(time.RFC3339, latest.UpdatedAt)
	if parseErr != nil {
		updatedAt = time.Now()
	}
	timeSinceUpdate := time.Since(updatedAt)

	// Update state even if not sending notification
	if isNewID || isNewStatus {
		c.SM.Set(project.ID, model.State{LastID: latest.ID, LastStatus: latest.Status})
		c.SM.Save()
	}

	// Logic for notification eligibility
	validStatuses := map[string]bool{"success": true, "failed": true, "running": true, "pending": true}
	if (isNewID || isNewStatus) && validStatuses[latest.Status] && timeSinceUpdate.Minutes() <= 30 {
		statusIcon := getStatusIcon(latest.Status)
		commitMsg, _ := c.FetchCommitDetail(project.ID, latest.SHA)
		userMsg, _ := c.FetchPipelineUser(project.ID, latest.ID)

		msgText := fmt.Sprintf("%s <b>Deploy Update!</b>\n\n<b>Repo:</b> %s\n<b>Branch:</b> %s\n<b>Status:</b> %s\n<b>Commit:</b> %s\n<b>By:</b> %s\n\n🔗 <a href=\"%s\">Buka GitLab Pipeline</a>",
			statusIcon, project.Name, latest.Ref, latest.Status, commitMsg, userMsg, latest.WebURL)

		return msgText, true
	}

	return "", false
}

func getStatusIcon(status string) string {
	switch status {
	case "success":
		return "✅"
	case "failed":
		return "❌"
	case "running":
		return "⏳"
	case "pending":
		return "🕘"
	default:
		return "🔄"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
