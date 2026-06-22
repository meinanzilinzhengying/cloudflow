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

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body for the OpenAI-compatible chat completion API.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// ChatResponse is the response from the chat completion API (OpenAI-compatible).
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Client is an HTTP client for interacting with LLM providers (OpenAI-compatible and Ollama).
type Client struct {
	httpClient   *http.Client
	models       map[string]config.LLMModel
	defaultModel string
	ollamaURL    string
}

// NewClient creates a new LLM client from the application configuration.
func NewClient(cfg *config.Config) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		models: make(map[string]config.LLMModel),
	}
	c.SetModels(cfg.AI.LLM.Models)
	c.defaultModel = cfg.AI.LLM.DefaultModel
	c.ollamaURL = cfg.AI.LLM.OllamaURL
	return c
}

// SetModels populates the internal models map from a slice of LLMModel configs.
func (c *Client) SetModels(models []config.LLMModel) {
	c.models = make(map[string]config.LLMModel, len(models))
	for _, m := range models {
		c.models[m.Name] = m
	}
}

// ListModels returns the names of all configured models.
func (c *Client) ListModels() []string {
	names := make([]string, 0, len(c.models))
	for name := range c.models {
		names = append(names, name)
	}
	return names
}

// Chat routes the request to the appropriate provider (OpenAI-compatible or Ollama).
func (c *Client) Chat(modelName string, messages []ChatMessage) (*ChatResponse, error) {
	model, ok := c.models[modelName]
	if !ok {
		return nil, fmt.Errorf("model %q not found in configuration", modelName)
	}

	if model.Provider == "ollama" {
		return c.ollamaChat(model, messages)
	}
	return c.openaiChat(model, messages)
}

// openaiChat sends a chat request to an OpenAI-compatible endpoint.
func (c *Client) openaiChat(model config.LLMModel, messages []ChatMessage) (*ChatResponse, error) {
	apiURL := model.APIURL
	if apiURL == "" {
		return nil, fmt.Errorf("api_url is required for provider %q", model.Provider)
	}

	chatReq := ChatRequest{
		Model:       model.Name,
		Messages:    messages,
		MaxTokens:   model.MaxTokens,
		Temperature: model.Temperature,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	endpoint := apiURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if model.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+model.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// ollamaChatRequest is the request body for the Ollama chat API.
type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaChatResponse is the response from the Ollama chat API (/api/chat).
type ollamaChatResponse struct {
	Model     string      `json:"model"`
	CreatedAt string      `json:"created_at"`
	Message   ChatMessage `json:"message"`
	Done      bool        `json:"done"`
}

// ollamaChat sends a chat request to an Ollama instance.
func (c *Client) ollamaChat(model config.LLMModel, messages []ChatMessage) (*ChatResponse, error) {
	apiURL := model.APIURL
	if apiURL == "" {
		apiURL = c.ollamaURL
	}

	ollamaReq := ollamaChatRequest{
		Model:    model.Name,
		Messages: messages,
		Stream:   false,
		Options: ollamaOptions{
			Temperature: model.Temperature,
		},
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama chat request: %w", err)
	}

	endpoint := apiURL + "/api/chat"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for Ollama: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to Ollama at %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	// Map the Ollama response to the standard ChatResponse format.
	chatResp := &ChatResponse{
		ID:      fmt.Sprintf("ollama-%d", time.Now().UnixMilli()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   ollamaResp.Model,
	}

	chatResp.Choices = []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{
		{
			Index: 0,
			Message: struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{
				Role:    ollamaResp.Message.Role,
				Content: ollamaResp.Message.Content,
			},
			FinishReason: "stop",
		},
	}

	if !ollamaResp.Done {
		chatResp.Choices[0].FinishReason = "unknown"
	}

	return chatResp, nil
}

// TestConnection tests if a provider API is reachable and working.
// For "ollama" provider it calls GET /api/tags; for others it sends a minimal
// chat completion request.
func (c *Client) TestConnection(provider, apiURL, apiKey, modelName string) (string, error) {
	switch provider {
	case "ollama":
		return c.testOllamaConnection(apiURL)
	default:
		return c.testOpenAIConnection(apiURL, apiKey, modelName)
	}
}

// testOllamaConnection verifies that an Ollama instance is reachable via /api/tags.
func (c *Client) testOllamaConnection(apiURL string) (string, error) {
	if apiURL == "" {
		apiURL = c.ollamaURL
	}

	endpoint := apiURL + "/api/tags"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach Ollama at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama returned status %d at %s", resp.StatusCode, apiURL)
	}

	return "Ollama 连接成功", nil
}

// testOpenAIConnection verifies an OpenAI-compatible endpoint by sending a simple message.
func (c *Client) testOpenAIConnection(apiURL, apiKey, modelName string) (string, error) {
	if apiURL == "" {
		return "", fmt.Errorf("api_url is required")
	}
	if modelName == "" {
		return "", fmt.Errorf("model name is required")
	}

	testMessages := []ChatMessage{
		{Role: "user", Content: "hi"},
	}

	chatReq := ChatRequest{
		Model:       modelName,
		Messages:    testMessages,
		MaxTokens:   1,
		Temperature: 0.0,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal test request: %w", err)
	}

	endpoint := apiURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach provider at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return fmt.Sprintf("连接成功（%s）", modelName), nil
}

// ollamaTagsResponse is the JSON response from GET /api/tags on an Ollama instance.
type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

type ollamaTagModel struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
}

// DiscoverOllamaModels lists all available models from an Ollama instance.
func (c *Client) DiscoverOllamaModels(ollamaURL string) ([]string, error) {
	if ollamaURL == "" {
		ollamaURL = c.ollamaURL
	}

	endpoint := ollamaURL + "/api/tags"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discover request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at %s: %w", ollamaURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d at %s", resp.StatusCode, ollamaURL)
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama tags response: %w", err)
	}

	modelNames := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		modelNames = append(modelNames, m.Name)
	}

	return modelNames, nil
}
