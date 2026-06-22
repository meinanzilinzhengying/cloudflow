package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/config"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/diagnosis"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/llm"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/nlq"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/prediction"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/rca"
	"github.com/meinanzilinzhengying/cloudflow/ai/pkg/logger"
)

type Server struct {
	llmClient       *llm.Client
	log             *logger.Logger
	config          *config.Config
	analysisHistory []AnalysisRecord

	// P6: AI engines
	rcaEngine        *rca.RCAEngine
	diagnosisEngine  *diagnosis.DiagnosisEngine
	predictionEngine *prediction.PredictionEngine
	nlqEngine        *nlq.NLQEngine
}

type AnalysisRecord struct {
	ID        string    `json:"id"`
	Query     string    `json:"query"`
	Result    string    `json:"result"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

type AnalyzeRequest struct {
	Query string `json:"query"`
	Model string `json:"model,omitempty"`
}

type AnalyzeResponse struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Model  string `json:"model"`
}

// LLMModelConfig represents a model configuration for API
type LLMModelConfig struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	APIURL      string  `json:"api_url"`
	APIKey      string  `json:"api_key"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
}

type ModelsConfigRequest struct {
	Models []LLMModelConfig `json:"models"`
}

type TestConnectionRequest struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	APIURL      string  `json:"api_url"`
	APIKey      string  `json:"api_key"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
}

type TestConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func NewServer(cfg *config.Config, log *logger.Logger, llmClient *llm.Client) *Server {
	s := &Server{
		config:           cfg,
		log:              log,
		llmClient:        llmClient,
		analysisHistory:  []AnalysisRecord{},
		rcaEngine:        rca.NewRCAEngine(),
		diagnosisEngine:  diagnosis.NewDiagnosisEngine(),
		predictionEngine: prediction.NewPredictionEngine(),
		nlqEngine:        nlq.NewNLQEngine(),
	}
	s.rcaEngine.LoadBuiltinPatterns(&cfg.AI.RCA)
	s.predictionEngine.AlertThresholdFactor = cfg.AI.Analysis.AlertThresholdFactor
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", s.Health)

	// Models API
	mux.HandleFunc("/api/v1/models", s.ListModels)
	mux.HandleFunc("/api/v1/analyze", s.Analyze)
	mux.HandleFunc("/api/v1/history", s.GetHistory)

	// New Ollama API
	mux.HandleFunc("/api/v1/ollama/models", s.GetOllamaModels)
	mux.HandleFunc("/api/v1/ollama/test", s.TestOllama)

	// Model configuration API
	mux.HandleFunc("/api/v1/config/models", s.HandleConfigModels)

	// Test connection API
	mux.HandleFunc("/api/v1/test", s.TestConnection)

	// Chat completions (OpenAI-compatible)
	mux.HandleFunc("/api/v1/chat/completions", s.ChatCompletions)

	// P6: AI analysis routes
	mux.HandleFunc("/api/v1/rca/analyze", s.RCAAnalyze)
	mux.HandleFunc("/api/v1/diagnosis", s.Diagnose)
	mux.HandleFunc("/api/v1/diagnosis/autofix", s.AutoFix)
	mux.HandleFunc("/api/v1/diagnosis/knowledge", s.GetKnowledge)
	mux.HandleFunc("/api/v1/prediction", s.Predict)
	mux.HandleFunc("/api/v1/prediction/capacity", s.PredictCapacity)
	mux.HandleFunc("/api/v1/prediction/failure", s.PredictFailure)
	mux.HandleFunc("/api/v1/nlq", s.NLQConvert)
	mux.HandleFunc("/api/v1/nlq/schemas", s.NLQListSchemas)

	return withCORS(mux)
}

// CORS middleware
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	type ModelInfo struct {
		Name        string  `json:"name"`
		Provider    string  `json:"provider"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}
	models := []ModelInfo{}
	for _, m := range s.config.AI.LLM.Models {
		models = append(models, ModelInfo{
			Name:        m.Name,
			Provider:    m.Provider,
			MaxTokens:   m.MaxTokens,
			Temperature: m.Temperature,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models":  models,
		"default": s.config.AI.LLM.DefaultModel,
	})
}

func (s *Server) Analyze(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	model := req.Model
	if model == "" {
		model = s.config.AI.LLM.DefaultModel
	}

	messages := []llm.ChatMessage{
		{
			Role: "system",
			Content: `你是一位专业的网络流量分析专家，你需要帮助用户分析和解释网络流量数据。
你的任务包括：
1. 解释网络流量异常现象
2. 提供流量优化建议
3. 分析网络性能瓶颈
4. 检测潜在的安全问题
5. 提供数据洞察和报告

回答请用中文，简洁且专业。`,
		},
		{
			Role:    "user",
			Content: req.Query,
		},
	}

	result, err := s.llmClient.Chat(model, messages)
	if err != nil {
		s.log.Errorf("Analysis failed: %v", err)
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	record := AnalysisRecord{
		ID:        fmt.Sprintf("analysis-%d", time.Now().Unix()),
		Query:     req.Query,
		Result:    result.Choices[0].Message.Content,
		Model:     model,
		CreatedAt: time.Now(),
	}
	s.analysisHistory = append(s.analysisHistory, record)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AnalyzeResponse{
		ID:     record.ID,
		Result: result.Choices[0].Message.Content,
		Model:  model,
	})
}

func (s *Server) GetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"history": s.analysisHistory,
	})
}

// GetOllamaModels fetches models from Ollama instance
func (s *Server) GetOllamaModels(w http.ResponseWriter, r *http.Request) {
	ollamaURL := r.URL.Query().Get("url")
	if ollamaURL == "" {
		ollamaURL = s.config.AI.LLM.OllamaURL
	}

	// Validate URL to prevent SSRF
	if !strings.HasPrefix(ollamaURL, "http://localhost") &&
		!strings.HasPrefix(ollamaURL, "http://127.0.0.1") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Only localhost Ollama instances are supported",
		})
		return
	}

	resp, err := http.Get(ollamaURL + "/api/tags")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Ollama request failed: %s", string(body)),
		})
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// TestOllama tests connection to Ollama
func (s *Server) TestOllama(w http.ResponseWriter, r *http.Request) {
	ollamaURL := r.URL.Query().Get("url")
	if ollamaURL == "" {
		ollamaURL = s.config.AI.LLM.OllamaURL
	}

	resp, err := http.Get(ollamaURL + "/api/tags")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("HTTP %d", resp.StatusCode),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TestConnectionResponse{
		Success: true,
		Message: "Ollama connection successful",
	})
}

// HandleConfigModels handles GET/POST for model configurations
func (s *Server) HandleConfigModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.GetConfigModels(w, r)
	case "POST":
		s.SaveConfigModels(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GetConfigModels returns current model configurations
func (s *Server) GetConfigModels(w http.ResponseWriter, r *http.Request) {
	models := []LLMModelConfig{}
	for _, m := range s.config.AI.LLM.Models {
		models = append(models, LLMModelConfig{
			Name:        m.Name,
			Provider:    m.Provider,
			APIURL:      m.APIURL,
			APIKey:      maskAPIKey(m.APIKey),
			MaxTokens:   m.MaxTokens,
			Temperature: m.Temperature,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models":        models,
		"default_model": s.config.AI.LLM.DefaultModel,
	})
}

// SaveConfigModels saves model configurations
func (s *Server) SaveConfigModels(w http.ResponseWriter, r *http.Request) {
	var req ModelsConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate models
	for _, model := range req.Models {
		if model.Name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Model name is required",
			})
			return
		}
		if model.Provider == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Model provider is required",
			})
			return
		}
	}

	// Update config
	newModels := []config.LLMModel{}
	for _, m := range req.Models {
		newModels = append(newModels, config.LLMModel{
			Name:        m.Name,
			Provider:    m.Provider,
			APIURL:      m.APIURL,
			APIKey:      m.APIKey,
			MaxTokens:   m.MaxTokens,
			Temperature: m.Temperature,
		})
	}
	s.config.AI.LLM.Models = newModels

	// Update LLM client models
	s.llmClient.SetModels(newModels)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Models saved successfully",
	})
}

// TestConnection tests connection to a model
func (s *Server) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req TestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// For Ollama, test differently
	if req.Provider == "ollama" {
		apiURL := req.APIURL
		if apiURL == "" {
			apiURL = s.config.AI.LLM.OllamaURL
		}
		resp, err := http.Get(apiURL + "/api/tags")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TestConnectionResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TestConnectionResponse{
				Success: false,
				Error:   fmt.Sprintf("HTTP %d", resp.StatusCode),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: true,
			Message: "Ollama connection successful",
		})
		return
	}

	// For other providers, test with a simple request
	testReq, err := http.NewRequest("GET", req.APIURL+"/models", nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if req.APIKey != "" {
		testReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(testReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestConnectionResponse{
			Success: true,
			Message: "Connection successful",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TestConnectionResponse{
		Success: false,
		Error:   fmt.Sprintf("HTTP %d", resp.StatusCode),
	})
}

// ChatCompletions handles OpenAI-compatible chat completions
func (s *Server) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		req.Model = s.config.AI.LLM.DefaultModel
	}

	if len(req.Messages) == 0 {
		http.Error(w, "Messages are required", http.StatusBadRequest)
		return
	}

	// Convert to LLM messages
	messages := make([]llm.ChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Call LLM
	result, err := s.llmClient.Chat(req.Model, messages)
	if err != nil {
		s.log.Errorf("Chat failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return OpenAI-compatible response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      "chatcmpl-" + fmt.Sprintf("%d", time.Now().Unix()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": result.Choices[0].Message.Content,
				},
				"finish_reason": "stop",
			},
		},
	})
}

// maskAPIKey masks API key for security
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// ============================================================================
// P6: RCA Root Cause Analysis
// ============================================================================

type RCAAnalyzeRequest struct {
	Anomalies []*rca.AnomalyEvent `json:"anomalies"`
}

func (s *Server) RCAAnalyze(w http.ResponseWriter, r *http.Request) {
	var req RCAAnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.rcaEngine.Analyze(req.Anomalies))
}

// ============================================================================
// P6: Auto Diagnosis
// ============================================================================

type DiagnoseRequest struct {
	Service   string              `json:"service"`
	AlertType string              `json:"alert_type"`
	Symptoms  []diagnosis.Symptom `json:"symptoms"`
}

func (s *Server) Diagnose(w http.ResponseWriter, r *http.Request) {
	var req DiagnoseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.diagnosisEngine.Diagnose(req.Service, req.AlertType, req.Symptoms))
}

type AutoFixRequest struct {
	Service   string              `json:"service"`
	AlertType string              `json:"alert_type"`
	Symptoms  []diagnosis.Symptom `json:"symptoms"`
}

func (s *Server) AutoFix(w http.ResponseWriter, r *http.Request) {
	var req AutoFixRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	result := s.diagnosisEngine.Diagnose(req.Service, req.AlertType, req.Symptoms)
	fixResult := s.diagnosisEngine.AutoFix(req.Service, result)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"diagnosis":       result,
		"auto_fix_result": fixResult,
	})
}

func (s *Server) GetKnowledge(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	entries := s.diagnosisEngine.GetKnowledge(category)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"total":   len(entries),
	})
}

// ============================================================================
// P6: Predictive Analysis
// ============================================================================

type PredictRequest struct {
	Key      string  `json:"key"`
	Horizon  string  `json:"horizon"`
	Resource string  `json:"resource,omitempty"`
	Limit    float64 `json:"limit,omitempty"`
}

func (s *Server) Predict(w http.ResponseWriter, r *http.Request) {
	var req PredictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	horizon, err := time.ParseDuration(req.Horizon)
	if err != nil {
		horizon = time.Hour
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.predictionEngine.Predict(req.Key, horizon))
}

type PredictCapacityRequest struct {
	Resource string  `json:"resource"`
	Limit    float64 `json:"limit"`
}

func (s *Server) PredictCapacity(w http.ResponseWriter, r *http.Request) {
	var req PredictCapacityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.predictionEngine.PredictCapacity(req.Resource, req.Limit))
}

type PredictFailureRequest struct {
	Service    string                     `json:"service"`
	Indicators []prediction.RiskIndicator `json:"indicators"`
}

func (s *Server) PredictFailure(w http.ResponseWriter, r *http.Request) {
	var req PredictFailureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.predictionEngine.PredictFailure(req.Service, req.Indicators))
}

// ============================================================================
// P6: NLQ Natural Language Query
// ============================================================================

type NLQConvertRequest struct {
	Query string `json:"query"`
}

func (s *Server) NLQConvert(w http.ResponseWriter, r *http.Request) {
	var req NLQConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.nlqEngine.Convert(req.Query))
}

func (s *Server) NLQListSchemas(w http.ResponseWriter, r *http.Request) {
	schemas := s.nlqEngine.ListSchemas()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"schemas": schemas,
		"total":   len(schemas),
	})
}
