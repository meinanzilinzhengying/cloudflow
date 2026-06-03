// Package topology 拓扑API模块
// 提供RESTful API接口查询拓扑数据
package topology

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloud-flow-agent/internal/network"
)

// APIConfig API配置
type APIConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	ListenAddr   string `json:"listen_addr" yaml:"listen_addr"`
	AuthEnabled  bool   `json:"auth_enabled" yaml:"auth_enabled"`
	AuthToken    string `json:"auth_token" yaml:"auth_token"`
	RateLimit    int    `json:"rate_limit" yaml:"rate_limit"`
	ReadTimeout  int    `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout int    `json:"write_timeout" yaml:"write_timeout"`
}

// TopologyAPI 拓扑API服务
type TopologyAPI struct {
	config       *APIConfig
	discovery    *DiscoveryEngine
	tracer       *PathTracer
	storage      *TopologyStorage
	fullstack    *FullStackManager
	fsHandler    *FullStackHandler
	appTopology  *AppTopologyManager
	appHandler   *AppTopologyHandler
	netAnalysis  *network.AnalysisManager
	netHandler   *network.AnalysisHandler
	server       *http.Server
}

// NewTopologyAPI 创建拓扑API服务
func NewTopologyAPI(config *APIConfig, discovery *DiscoveryEngine, tracer *PathTracer, storage *TopologyStorage) *TopologyAPI {
	// 创建全栈拓扑管理器
	fullstackManager := NewFullStackManager(discovery, tracer, storage)
	fsHandler := NewFullStackHandler(fullstackManager)
	
	// 创建应用拓扑管理器
	appTopologyManager := NewAppTopologyManager(discovery, tracer, storage)
	appHandler := NewAppTopologyHandler(appTopologyManager)
	
	// 创建网络分析管理器
	netAnalysisManager := network.NewAnalysisManager()
	netHandler := network.NewAnalysisHandler(netAnalysisManager)
	
	return &TopologyAPI{
		config:      config,
		discovery:   discovery,
		tracer:      tracer,
		storage:     storage,
		fullstack:   fullstackManager,
		fsHandler:   fsHandler,
		appTopology: appTopologyManager,
		appHandler:  appHandler,
		netAnalysis: netAnalysisManager,
		netHandler:  netHandler,
	}
}

// Start 启动API服务
func (a *TopologyAPI) Start() error {
	if !a.config.Enabled {
		return nil
	}
	
	mux := http.NewServeMux()
	
	// 注册路由
	mux.HandleFunc("/api/v1/topology/entities", a.authMiddleware(a.handleEntities))
	mux.HandleFunc("/api/v1/topology/entities/", a.authMiddleware(a.handleEntity))
	mux.HandleFunc("/api/v1/topology/relations", a.authMiddleware(a.handleRelations))
	mux.HandleFunc("/api/v1/topology/relations/", a.authMiddleware(a.handleRelation))
	mux.HandleFunc("/api/v1/topology/paths", a.authMiddleware(a.handlePaths))
	mux.HandleFunc("/api/v1/topology/paths/", a.authMiddleware(a.handlePath))
	mux.HandleFunc("/api/v1/topology/graph", a.authMiddleware(a.handleGraph))
	mux.HandleFunc("/api/v1/topology/stats", a.authMiddleware(a.handleStats))
	mux.HandleFunc("/api/v1/topology/search", a.authMiddleware(a.handleSearch))
	mux.HandleFunc("/api/v1/topology/trace/", a.authMiddleware(a.handleTrace))
	mux.HandleFunc("/api/v1/topology/dependencies/", a.authMiddleware(a.handleDependencies))
	
	// 全栈拓扑API
	a.fsHandler.RegisterRoutes(mux)
	
	// 应用拓扑API
	a.appHandler.RegisterRoutes(mux)
	
	// 网络分析API
	a.netHandler.RegisterRoutes(mux)
	
	// 静态文件服务 - 拓扑可视化页面
	mux.HandleFunc("/topology", a.handleTopologyPage)
	mux.HandleFunc("/topology/", a.handleTopologyPage)
	mux.HandleFunc("/app-topology", a.handleAppTopologyPage)
	mux.HandleFunc("/app-topology/", a.handleAppTopologyPage)
	
	mux.HandleFunc("/api/v1/health", a.handleHealth)
	
	a.server = &http.Server{
		Addr:         a.config.ListenAddr,
		Handler:      mux,
		ReadTimeout:  time.Duration(a.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(a.config.WriteTimeout) * time.Second,
	}
	
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("API server error: %v\n", err)
		}
	}()
	
	return nil
}

// Stop 停止API服务
func (a *TopologyAPI) Stop() error {
	if a.server != nil {
		ctx, cancel := contextWithTimeout(5 * time.Second)
		defer cancel()
		return a.server.Shutdown(ctx)
	}
	return nil
}

// contextWithTimeout 创建带超时的context
func contextWithTimeout(timeout time.Duration) (interface{ Done() <-chan struct{} }, func()) {
	type cancelCtx struct {
		done chan struct{}
	}
	ctx := &cancelCtx{done: make(chan struct{})}
	cancel := func() {
		close(ctx.done)
	}
	go func() {
		time.Sleep(timeout)
		cancel()
	}()
	return ctx, cancel
}

// authMiddleware 认证中间件
func (a *TopologyAPI) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.config.AuthEnabled {
			token := r.Header.Get("Authorization")
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			
			if token == "" || token != "Bearer "+a.config.AuthToken {
				a.respondError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		
		next(w, r)
	}
}

// handleEntities 处理实体列表请求
func (a *TopologyAPI) handleEntities(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listEntities(w, r)
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listEntities 列出实体
func (a *TopologyAPI) listEntities(w http.ResponseWriter, r *http.Request) {
	// 解析参数
	entityType := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	
	var entities []*Entity
	var err error
	
	if a.storage != nil && a.storage.config.Enabled {
		if entityType != "" {
			entities, err = a.storage.GetEntitiesByType(EntityType(entityType), limit, offset)
		} else {
			entities, err = a.storage.GetEntities(limit, offset)
		}
	} else if a.discovery != nil {
		if entityType != "" {
			entities = a.discovery.GetEntitiesByType(EntityType(entityType))
		} else {
			entities = a.discovery.GetEntities()
		}
	}
	
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	a.respondJSON(w, http.StatusOK, map[string]interface{}{
		"entities": entities,
		"count":    len(entities),
	})
}

// handleEntity 处理单个实体请求
func (a *TopologyAPI) handleEntity(w http.ResponseWriter, r *http.Request) {
	// 提取实体ID
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/entities/")
	
	switch r.Method {
	case http.MethodGet:
		a.getEntity(w, r, id)
	case http.MethodDelete:
		a.deleteEntity(w, r, id)
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// getEntity 获取实体
func (a *TopologyAPI) getEntity(w http.ResponseWriter, r *http.Request, id string) {
	var entity *Entity
	var err error
	
	if a.storage != nil && a.storage.config.Enabled {
		entity, err = a.storage.GetEntity(id)
	} else if a.discovery != nil {
		entity = a.discovery.GetEntity(id)
	}
	
	if err != nil || entity == nil {
		a.respondError(w, http.StatusNotFound, "entity not found")
		return
	}
	
	// 获取关联关系
	var relations []*Relation
	if a.storage != nil && a.storage.config.Enabled {
		relations, _ = a.storage.GetEntityRelations(id)
	} else if a.discovery != nil {
		relations = a.discovery.GetRelations()
		// 过滤相关关系
		var filtered []*Relation
		for _, rel := range relations {
			if rel.SourceID == id || rel.TargetID == id {
				filtered = append(filtered, rel)
			}
		}
		relations = filtered
	}
	
	a.respondJSON(w, http.StatusOK, map[string]interface{}{
		"entity":    entity,
		"relations": relations,
	})
}

// deleteEntity 删除实体
func (a *TopologyAPI) deleteEntity(w http.ResponseWriter, r *http.Request, id string) {
	if a.storage == nil || !a.storage.config.Enabled {
		a.respondError(w, http.StatusServiceUnavailable, "storage not enabled")
		return
	}
	
	if err := a.storage.DeleteEntity(id); err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	a.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleRelations 处理关系列表请求
func (a *TopologyAPI) handleRelations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listRelations(w, r)
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listRelations 列出关系
func (a *TopologyAPI) listRelations(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	
	var relations []*Relation
	var err error
	
	if a.storage != nil && a.storage.config.Enabled {
		relations, err = a.storage.GetRelations(limit, offset)
	} else if a.discovery != nil {
		relations = a.discovery.GetRelations()
	}
	
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	a.respondJSON(w, http.StatusOK, map[string]interface{}{
		"relations": relations,
		"count":     len(relations),
	})
}

// handleRelation 处理单个关系请求
func (a *TopologyAPI) handleRelation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/relations/")
	
	switch r.Method {
	case http.MethodGet:
		// 获取关系详情
		a.respondJSON(w, http.StatusOK, map[string]interface{}{
			"relation_id": id,
		})
	case http.MethodDelete:
		if a.storage != nil && a.storage.config.Enabled {
			if err := a.storage.DeleteRelation(id); err != nil {
				a.respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		a.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePaths 处理路径列表请求
func (a *TopologyAPI) handlePaths(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listPaths(w, r)
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listPaths 列出路径
func (a *TopologyAPI) listPaths(w http.ResponseWriter, r *http.Request) {
	traceID := r.URL.Query().Get("trace_id")
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	
	var paths []*TracePath
	var err error
	
	if a.storage != nil && a.storage.config.Enabled {
		if traceID != "" {
			paths, err = a.storage.GetPathsByTraceID(traceID)
		} else {
			paths, err = a.storage.GetPaths(limit, offset)
		}
	} else if a.tracer != nil {
		if traceID != "" {
			paths = a.tracer.GetPathsByTraceID(traceID)
		} else {
			paths = a.tracer.GetCompletedPaths(limit)
		}
	}
	
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// 按状态过滤
	if status != "" {
		var filtered []*TracePath
		for _, path := range paths {
			if path.Status == status {
				filtered = append(filtered, path)
			}
		}
		paths = filtered
	}
	
	a.respondJSON(w, http.StatusOK, map[string]interface{}{
		"paths": paths,
		"count": len(paths),
	})
}

// handlePath 处理单个路径请求
func (a *TopologyAPI) handlePath(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/paths/")
	
	switch r.Method {
	case http.MethodGet:
		a.getPath(w, r, id)
	case http.MethodDelete:
		if a.storage != nil && a.storage.config.Enabled {
			if err := a.storage.DeletePath(id); err != nil {
				a.respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		a.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// getPath 获取路径
func (a *TopologyAPI) getPath(w http.ResponseWriter, r *http.Request, id string) {
	var path *TracePath
	var err error
	
	if a.storage != nil && a.storage.config.Enabled {
		path, err = a.storage.GetPath(id)
	} else if a.tracer != nil {
		path = a.tracer.GetPath(id)
	}
	
	if err != nil || path == nil {
		a.respondError(w, http.StatusNotFound, "path not found")
		return
	}
	
	a.respondJSON(w, http.StatusOK, path)
}

// handleGraph 处理拓扑图请求
func (a *TopologyAPI) handleGraph(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.getGraph(w, r)
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// getGraph 获取拓扑图
func (a *TopologyAPI) getGraph(w http.ResponseWriter, r *http.Request) {
	var graph *TopologyGraph
	
	if a.tracer != nil {
		graph = a.tracer.BuildTopologyGraph()
	} else if a.discovery != nil {
		topology := a.discovery.GetTopology()
		graph = &TopologyGraph{
			Nodes: make(map[string]*TopologyNode),
			Edges: make(map[string]*TopologyEdge),
		}
		
		for id, entity := range topology.Entities {
			graph.Nodes[id] = &TopologyNode{
				ID:   entity.ID,
				Type: entity.Type,
				Name: entity.Name,
			}
		}
		
		for _, relation := range topology.Relations {
			edgeID := fmt.Sprintf("%s-%s", relation.SourceID, relation.TargetID)
			graph.Edges[edgeID] = &TopologyEdge{
				ID:       edgeID,
				SourceID: relation.SourceID,
				TargetID: relation.TargetID,
			}
		}
	}
	
	if graph == nil {
		graph = &TopologyGraph{
			Nodes: make(map[string]*TopologyNode),
			Edges: make(map[string]*TopologyEdge),
		}
	}
	
	a.respondJSON(w, http.StatusOK, graph)
}

// handleStats 处理统计请求
func (a *TopologyAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]interface{})
	
	if a.discovery != nil {
		topology := a.discovery.GetTopology()
		stats["entity_count"] = len(topology.Entities)
		stats["relation_count"] = len(topology.Relations)
	}
	
	if a.tracer != nil {
		pathStats := a.tracer.GetStats()
		stats["path_stats"] = pathStats
	}
	
	if a.storage != nil && a.storage.config.Enabled {
		storageStats, _ := a.storage.GetStats()
		stats["storage"] = storageStats
	}
	
	a.respondJSON(w, http.StatusOK, stats)
}

// handleSearch 处理搜索请求
func (a *TopologyAPI) handleSearch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.search(w, r)
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// search 搜索实体
func (a *TopologyAPI) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		a.respondError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	
	var entities []*Entity
	var err error
	
	if a.storage != nil && a.storage.config.Enabled {
		entities, err = a.storage.SearchEntities(query, limit)
	} else if a.discovery != nil {
		// 内存搜索
		allEntities := a.discovery.GetEntities()
		for _, entity := range allEntities {
			if strings.Contains(entity.Name, query) ||
				strings.Contains(entity.ID, query) ||
				strings.Contains(entity.Namespace, query) ||
				strings.Contains(entity.PodName, query) {
				entities = append(entities, entity)
				if len(entities) >= limit {
					break
				}
			}
		}
	}
	
	if err != nil {
		a.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	a.respondJSON(w, http.StatusOK, map[string]interface{}{
		"query":    query,
		"entities": entities,
		"count":    len(entities),
	})
}

// handleTrace 处理追踪请求
func (a *TopologyAPI) handleTrace(w http.ResponseWriter, r *http.Request) {
	traceID := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/trace/")
	
	switch r.Method {
	case http.MethodGet:
		var paths []*TracePath
		
		if a.storage != nil && a.storage.config.Enabled {
			paths, _ = a.storage.GetPathsByTraceID(traceID)
		} else if a.tracer != nil {
			paths = a.tracer.GetPathsByTraceID(traceID)
		}
		
		a.respondJSON(w, http.StatusOK, map[string]interface{}{
			"trace_id": traceID,
			"paths":    paths,
			"count":    len(paths),
		})
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleDependencies 处理依赖关系请求
func (a *TopologyAPI) handleDependencies(w http.ResponseWriter, r *http.Request) {
	entityID := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/dependencies/")
	
	switch r.Method {
	case http.MethodGet:
		var deps, dependents map[string]int64
		
		if a.tracer != nil {
			deps = a.tracer.GetEntityDependencies(entityID)
			dependents = a.tracer.GetEntityDependents(entityID)
		}
		
		a.respondJSON(w, http.StatusOK, map[string]interface{}{
			"entity_id":   entityID,
			"depends_on":  deps,
			"depended_by": dependents,
		})
	default:
		a.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleHealth 处理健康检查
func (a *TopologyAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"services": map[string]bool{
			"discovery": a.discovery != nil,
			"tracer":    a.tracer != nil,
			"storage":   a.storage != nil,
			"fullstack": a.fullstack != nil,
		},
	}
	
	a.respondJSON(w, http.StatusOK, status)
}

// handleTopologyPage 处理拓扑页面请求
func (a *TopologyAPI) handleTopologyPage(w http.ResponseWriter, r *http.Request) {
	// 读取并返回拓扑HTML页面
	http.ServeFile(w, r, "web/topology.html")
}

// handleAppTopologyPage 处理应用拓扑页面请求
func (a *TopologyAPI) handleAppTopologyPage(w http.ResponseWriter, r *http.Request) {
	// 读取并返回应用拓扑HTML页面
	http.ServeFile(w, r, "web/app-topology.html")
}

// respondJSON 返回JSON响应
func (a *TopologyAPI) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// respondError 返回错误响应
func (a *TopologyAPI) respondError(w http.ResponseWriter, statusCode int, message string) {
	a.respondJSON(w, statusCode, map[string]interface{}{
		"error": map[string]string{
			"code":    strconv.Itoa(statusCode),
			"message": message,
		},
	})
}

// APIResponse API响应
type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// APIError API错误
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// EntityListResponse 实体列表响应
type EntityListResponse struct {
	Entities []*Entity `json:"entities"`
	Count    int       `json:"count"`
	Total    int       `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// PathListResponse 路径列表响应
type PathListResponse struct {
	Paths  []*TracePath `json:"paths"`
	Count  int          `json:"count"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// GraphResponse 拓扑图响应
type GraphResponse struct {
	Nodes []*TopologyNode `json:"nodes"`
	Edges []*TopologyEdge `json:"edges"`
}

// StatsResponse 统计响应
type StatsResponse struct {
	EntityCount    int64  `json:"entity_count"`
	RelationCount  int64  `json:"relation_count"`
	PathCount      int64  `json:"path_count"`
	ActivePaths    int64  `json:"active_paths"`
	SuccessPaths   int64  `json:"success_paths"`
	FailedPaths    int64  `json:"failed_paths"`
	AvgPathHops    float64 `json:"avg_path_hops"`
	AvgPathLatency float64 `json:"avg_path_latency_ms"`
}

// DependencyResponse 依赖关系响应
type DependencyResponse struct {
	EntityID   string            `json:"entity_id"`
	DependsOn  map[string]int64  `json:"depends_on"`
	DependedBy map[string]int64  `json:"depended_by"`
}
