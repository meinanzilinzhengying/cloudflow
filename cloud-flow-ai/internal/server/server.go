package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/config"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/llm"
	"github.com/meinanzilinzhengying/cloudflow/ai/pkg/logger"
)

// AnalysisRecord stores the result of a traffic analysis request.
type AnalysisRecord struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	ModelUsed string    `json:"model_used"`
}

// Server is the HTTP server for the CloudFlow AI service.
type Server struct {
	llmClient       *llm.Client
	log             *logger.Logger
	cfg             *config.Config
	analysisHistory []AnalysisRecord
	mu              sync.RWMutex
}

// NewServer creates a new Server with the given dependencies.
// Parameters: cfg, log, llmClient (matching existing main.go call signature).
func NewServer(cfg *config.Config, log *logger.Logger, llmClient *llm.Client) *Server {
	return &Server{
		llmClient:       llmClient,
		log:             log,
		cfg:             cfg,
		analysisHistory: make([]AnalysisRecord, 0),
	}
}

// corsMiddleware wraps an http.Handler with CORS headers.
func corsMiddleware(next http.Handler) http.Handler {
	// 从环境变量读取允许的来源，逗号分隔
	allowedOrigins := strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")
	if len(allowedOrigins) == 0 || allowedOrigins[0] == "" {
		// 默认值：本地开发环境
		allowedOrigins = []string{"http://localhost:3000", "http://localhost:8080"}
	}

	// 构建来源映射用于快速查找
	originMap := make(map[string]bool)
	for _, origin := range allowedOrigins {
		originMap[strings.TrimSpace(origin)] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originMap[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Existing routes
	mux.HandleFunc("GET /health", s.Health)
	mux.HandleFunc("GET /api/v1/models", s.GetModels)
	mux.HandleFunc("POST /api/v1/analyze", s.Analyze)
	mux.HandleFunc("GET /api/v1/history", s.GetHistory)

	// New configuration routes
	mux.HandleFunc("GET /api/v1/config/models", s.GetConfigModels)
	mux.HandleFunc("POST /api/v1/config/models", s.SaveConfigModels)

	// New connection test route
	mux.HandleFunc("POST /api/v1/test", s.TestConnection)

	// New Ollama discovery route
	mux.HandleFunc("GET /api/v1/ollama/models", s.GetOllamaModels)

	return corsMiddleware(mux)
}

// Health returns a simple health-check response.
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetModels returns the list of available model names from the LLM client.
func (s *Server) GetModels(w http.ResponseWriter, r *http.Request) {
	models := s.llmClient.ListModels()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"models": models})
}

// Analyze accepts a traffic analysis prompt and returns the AI-generated response.
func (s *Server) Analyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model   string `json:"model"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, `{"error":"content is required"}`, http.StatusBadRequest)
		return
	}

	modelName := req.Model
	if modelName == "" {
		modelName = s.cfg.AI.LLM.DefaultModel
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: "You are a network traffic analysis expert. Analyze the provided data and return structured insights."},
		{Role: "user", Content: req.Content},
	}

	resp, err := s.llmClient.Chat(modelName, messages)
	if err != nil {
		s.log.Error("LLM chat failed", "error", err)
		http.Error(w, `{"error":"analysis request failed"}`, http.StatusInternalServerError)
		return
	}

	responseText := ""
	if len(resp.Choices) > 0 {
		responseText = resp.Choices[0].Message.Content
	}

	record := AnalysisRecord{
		ID:        resp.ID,
		Timestamp: time.Now(),
		Prompt:    req.Content,
		Response:  responseText,
		ModelUsed: modelName,
	}

	s.mu.Lock()
	s.analysisHistory = append(s.analysisHistory, record)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetHistory returns the full analysis history.
func (s *Server) GetHistory(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	history := make([]AnalysisRecord, len(s.analysisHistory))
	copy(history, s.analysisHistory)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"history": history,
		"total":   len(history),
	})
}

// modelResponse is a safe representation of a model config without the API key.
type modelResponse struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	APIURL      string  `json:"api_url"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	IsDefault   bool    `json:"is_default"`
}

// GetConfigModels returns all configured models without exposing API keys.
func (s *Server) GetConfigModels(w http.ResponseWriter, r *http.Request) {
	models := s.cfg.AI.LLM.Models
	defaultModel := s.cfg.AI.LLM.DefaultModel

	respModels := make([]modelResponse, 0, len(models))
	for _, m := range models {
		respModels = append(respModels, modelResponse{
			Name:        m.Name,
			Provider:    m.Provider,
			APIURL:      m.APIURL,
			MaxTokens:   m.MaxTokens,
			Temperature: m.Temperature,
			IsDefault:   m.Name == defaultModel,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models":  respModels,
		"default": defaultModel,
	})
}

// SaveConfigModels updates the in-memory model configuration.
func (s *Server) SaveConfigModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Models  []config.LLMModel `json:"models"`
		Default string            `json:"default"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Default != "" {
		s.cfg.AI.LLM.DefaultModel = req.Default
	}
	if req.Models != nil {
		s.cfg.AI.LLM.Models = req.Models
		s.llmClient.SetModels(req.Models)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// TestConnection tests connectivity to a model API provider.
func (s *Server) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		APIURL   string `json:"api_url"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"result":  "",
			"error":   "invalid request body",
		})
		return
	}

	result, err := s.llmClient.TestConnection(req.Provider, req.APIURL, req.APIKey, req.Model)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]interface{}{
		"success": err == nil,
		"result":  result,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	json.NewEncoder(w).Encode(resp)
}

// GetOllamaModels discovers models from a local Ollama instance.
func (s *Server) GetOllamaModels(w http.ResponseWriter, r *http.Request) {
	ollamaURL := r.URL.Query().Get("url")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	models, err := s.llmClient.DiscoverOllamaModels(ollamaURL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]interface{}{
		"success": err == nil,
		"models":  models,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	json.NewEncoder(w).Encode(resp)
}
