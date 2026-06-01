// Package mixserver 提供数据统计计算服务
// 定期扫描存储数据，生成统计摘要
package mixserver

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"cloud-flow-center/internal/storage"
	"cloud-flow-center/pkg/logger"
)

// Analyzer 数据分析器
type Analyzer struct {
	mu            sync.RWMutex
	db            *sql.DB
	logger        *logger.Logger
	stopCh        chan struct{}
	wg            sync.WaitGroup
	summaries     map[string]*DaySummary
	stopped       sync.Once
	allowedOrigins []string
}

// DaySummary 每日数据摘要
type DaySummary struct {
	Date         string  `json:"date"`
	MetricsCount int     `json:"metrics_count"`
	TracesCount  int     `json:"traces_count"`
	ProfsCount   int     `json:"profiling_count"`
	NodeCount    int     `json:"node_count"`
	AvgCPU       float64 `json:"avg_cpu_percent"`
	AvgMemory    float64 `json:"avg_memory_percent"`
	UpdatedAt    int64   `json:"updated_at"`
}

// NewAnalyzer 创建分析器
func NewAnalyzer(dbOrStore interface{}, log *logger.Logger, allowedOrigins []string) (*Analyzer, error) {
	var db *sql.DB
	switch v := dbOrStore.(type) {
	case *sql.DB:
		db = v
	case storage.StorageEngine:
		if dbEngine, ok := v.(interface{ GetDB() *sql.DB }); ok {
			db = dbEngine.GetDB()
		} else {
			log.Warnf("无法从 StorageEngine 获取 *sql.DB 实例")
		}
	}
	if db == nil {
		return nil, fmt.Errorf("无法获取有效的数据库连接，分析器初始化失败")
	}
	return &Analyzer{
		db:             db,
		logger:         log,
		stopCh:         make(chan struct{}),
		summaries:      make(map[string]*DaySummary),
		allowedOrigins: allowedOrigins,
	}, nil
}

// Start 启动定时分析
func (a *Analyzer) Start() {
	a.analyze()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.analyze()
			case <-a.stopCh:
				a.logger.Info("分析器已停止")
				return
			}
		}
	}()
	a.logger.Info("数据分析器已启动，每 5 分钟执行一次")
}

func (a *Analyzer) analyze() {
	today := time.Now().Format("2006-01-02")
	summary := &DaySummary{
		Date:      today,
		UpdatedAt: time.Now().Unix(),
	}

	var metricsCount int
	err := a.db.QueryRow("SELECT COUNT(*) FROM metrics WHERE DATE(ts) = ?", today).Scan(&metricsCount)
	if err != nil {
		a.logger.Warnf("统计 metrics 失败: %v", err)
		metricsCount = 0
	}
	summary.MetricsCount = metricsCount

	var tracesCount int
	err = a.db.QueryRow("SELECT COUNT(*) FROM traces WHERE DATE(ts) = ?", today).Scan(&tracesCount)
	if err != nil {
		a.logger.Warnf("统计 traces 失败: %v", err)
		tracesCount = 0
	}
	summary.TracesCount = tracesCount

	var profsCount int
	err = a.db.QueryRow("SELECT COUNT(*) FROM profiling WHERE DATE(ts) = ?", today).Scan(&profsCount)
	if err != nil {
		a.logger.Warnf("统计 profiling 失败: %v", err)
		profsCount = 0
	}
	summary.ProfsCount = profsCount

	var nodeCount int
	err = a.db.QueryRow("SELECT COUNT(*) FROM probes").Scan(&nodeCount)
	if err != nil {
		a.logger.Warnf("统计节点失败: %v", err)
		nodeCount = 0
	}
	summary.NodeCount = nodeCount

	summary.AvgCPU, summary.AvgMemory = a.calcAverages(today)

	a.mu.Lock()
	a.summaries[today] = summary
	a.mu.Unlock()

	a.logger.Infof("分析完成: %s, metrics=%d, traces=%d, nodes=%d, cpu=%.1f%%, mem=%.1f%%",
		today, summary.MetricsCount, summary.TracesCount, summary.NodeCount,
		summary.AvgCPU, summary.AvgMemory)
}

func (a *Analyzer) calcAverages(day string) (cpuAvg, memAvg float64) {
	var cpuSum float64
	var cpuN int
	queryCPU := `
		SELECT 
			COALESCE(SUM(cpu_usage), 0),
			COUNT(CASE WHEN cpu_usage IS NOT NULL THEN 1 END)
		FROM metrics 
		WHERE DATE(ts) = ?`
	err := a.db.QueryRow(queryCPU, day).Scan(&cpuSum, &cpuN)
	if err != nil {
		a.logger.Warnf("计算 CPU 平均值失败: %v", err)
		cpuSum = 0
		cpuN = 0
	}

	var memSum float64
	var memN int
	queryMem := `
		SELECT 
			COALESCE(SUM(memory_usage), 0),
			COUNT(CASE WHEN memory_usage IS NOT NULL THEN 1 END)
		FROM metrics 
		WHERE DATE(ts) = ?`
	err = a.db.QueryRow(queryMem, day).Scan(&memSum, &memN)
	if err != nil {
		a.logger.Warnf("计算内存平均值失败: %v", err)
		memSum = 0
		memN = 0
	}

	if cpuN > 0 {
		cpuAvg = cpuSum / float64(cpuN)
	}
	if memN > 0 {
		memAvg = memSum / float64(memN)
	}
	return
}

// GetSummaries 获取统计摘要
func (a *Analyzer) GetSummaries() map[string]*DaySummary {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make(map[string]*DaySummary)
	for k, v := range a.summaries {
		result[k] = v
	}
	return result
}

// Stop 停止分析器
func (a *Analyzer) Stop() {
	a.stopped.Do(func() {
		close(a.stopCh)
		a.wg.Wait()
	})
}

// isOriginAllowed 检查请求来源是否在允许的白名单中
func (a *Analyzer) isOriginAllowed(origin string) bool {
	for _, allowed := range a.allowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}

// SummaryHandler 返回 HTTP handler 用于查询摘要
func (a *Analyzer) SummaryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		origin := r.Header.Get("Origin")
		if origin != "" && a.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
		}

		summaries := a.GetSummaries()
		if err := json.NewEncoder(w).Encode(summaries); err != nil {
			a.logger.Errorf("SummaryHandler JSON Encode 失败: %v", err)
		}
	}
}
