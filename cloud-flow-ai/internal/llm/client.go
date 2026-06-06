package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/config"
)

type Client struct {
	httpClient *http.Client
	models     map[string]config.LLMModel
	defaultModel string
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens    int `json:"total_tokens"`
	} `json:"usage"`
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		defaultModel: cfg.AI.LLM.DefaultModel,
		models: make(map[string]config.LLMModel),
	}
}

func (c *Client) SetModels(models []config.LLMModel) {
	for _, m := range models {
		c.models[m.Name] = m
	}
}

func (c *Client) Chat(modelName string, messages []ChatMessage) (*ChatResponse, error) {
	model, ok := c.models[modelName]
	if !ok {
		if c.defaultModel != "" {
			model, ok = c.models[c.defaultModel]
			if !ok {
				return nil, fmt.Errorf("model %s not found", modelName)
			}
		} else {
			return nil, fmt.Errorf("model %s not found", modelName)
		}
	}

	reqBody := ChatRequest{
		Model: model.Name,
		Messages: messages,
		MaxTokens: model.MaxTokens,
		Temperature: model.Temperature,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %v", err)
	}

	httpReq, err := http.NewRequest("POST", model.APIURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+model.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %v", err)
	}

	return &chatResp, nil
}
