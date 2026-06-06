package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/config"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/llm"
	"github.com/meinanzilinzhengying/cloudflow/ai/pkg/logger"
)

type Server struct {
	llmClient *llm.Client
	log       *logger.Logger
	config    *config.Config
	analysisHistory []AnalysisRecord
}

type AnalysisRecord struct {
	ID         string      `json:"id"`
	Query      string      `json:"query"`
	Result     string      `json:"result"`
	Model      string      `json:"model"`
	CreatedAt  time.Time   `json:"created_at"`
}

type AnalyzeRequest struct {
	Query   string `json:"query"`
	Model   string `json:"model,omitempty"`
}

type AnalyzeResponse struct {
	ID       string `json:"id"`
	Result   string `json:"result"`
	Model    string `json:"model"`
}

func NewServer(cfg *config.Config, log *logger.Logger, llmClient *llm.Client) *Server {
	return &Server{
		config: cfg,
		log: log,
		llmClient: llmClient,
		analysisHistory: []AnalysisRecord{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.Health)
	mux.HandleFunc("GET /api/v1/models", s.ListModels)
	mux.HandleFunc("POST /api/v1/analyze", s.Analyze)
	mux.HandleFunc("GET /api/v1/history", s.GetHistory)

	return mux
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	type ModelInfo struct {
		Name          string  `json:"name"`
		Provider      string  `json:"provider"`
		MaxTokens     int     `json:"max_tokens"`
		Temperature   float64 `json:"temperature"`
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
		"models": models,
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
			Role: "user",
			Content: req.Query,
		},
	}

	resp, err := s.llmClient.Chat(model, messages)
	if err != nil {
		s.log.Error("Analysis failed", "error", err)
		http.Error(w, "Analysis failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	result := ""
	if len(resp.Choices) > 0 {
		result = resp.Choices[0].Message.Content
	}

	record := AnalysisRecord{
		ID:        generateID(),
		Query:     req.Query,
		Result:    result,
		Model:     model,
		CreatedAt: time.Now(),
	}

	s.analysisHistory = append(s.analysisHistory, record)
	if len(s.analysisHistory) > s.config.AI.Analysis.MaxAnalysisHistory {
		s.analysisHistory = s.analysisHistory[1:]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AnalyzeResponse{
		ID:      record.ID,
		Result:  result,
		Model:   model,
	})
}

func (s *Server) GetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"history": s.analysisHistory,
	})
}

func generateID() string {
	return time.Now().Format("20060102150405")
}
