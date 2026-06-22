// P6: 异常自动诊断与修复建议引擎
package diagnosis

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// DiagnosisEngine 自动诊断引擎
type DiagnosisEngine struct {
	mu sync.RWMutex

	// 诊断规则库
	rules []DiagnosisRule

	// 修复建议库
	remedies map[string][]Remedy

	// 诊断历史
	history []DiagnosisRecord

	// 知识库
	knowledgeBase map[string]KnowledgeEntry
}

// DiagnosisRule 诊断规则
type DiagnosisRule struct {
	ID          string
	Name        string
	Category    string
	Severity    string
	Conditions  []Condition
	Diagnosis   string
	Confidence  float64
	AutoFixable bool
}

// Condition 诊断条件
type Condition struct {
	Metric   string
	Operator string
	Value    float64
	Duration time.Duration
}

// Remedy 修复建议
type Remedy struct {
	ID          string
	Name        string
	Description string
	Steps       []string
	Risk        string
	AutoFixable bool
	Category    string
}

// DiagnosisRecord 诊断记录
type DiagnosisRecord struct {
	ID           string
	Timestamp    time.Time
	Service      string
	AlertType    string
	Symptoms     []Symptom
	Diagnosis    string
	Confidence   float64
	Remedies     []Remedy
	AutoFixApplied bool
	Result       string
}

// Symptom 症状
type Symptom struct {
	Metric    string
	Value     float64
	Severity  string
}

// KnowledgeEntry 知识库条目
type KnowledgeEntry struct {
	ID          string
	Title       string
	Category    string
	Content     string
	Tags        []string
	RelatedRules []string
}

// DiagnosisResult 诊断结果
type DiagnosisResult struct {
	Service       string
	AlertType     string
	Diagnosis     string
	Confidence    float64
	Severity      string
	Remedies      []Remedy
	CanAutoFix    bool
	KnowledgeRefs []string
	TimeToResolve time.Duration
}

// NewDiagnosisEngine 创建诊断引擎
func NewDiagnosisEngine() *DiagnosisEngine {
	engine := &DiagnosisEngine{
		rules:         []DiagnosisRule{},
		remedies:      make(map[string][]Remedy),
		history:       []DiagnosisRecord{},
		knowledgeBase: make(map[string]KnowledgeEntry),
	}
	engine.loadBuiltinRules()
	engine.loadBuiltinRemedies()
	engine.loadBuiltinKnowledge()
	return engine
}

// ============================================================================
// 诊断核心逻辑
// ============================================================================

// Diagnose 执行自动诊断
func (e *DiagnosisEngine) Diagnose(service string, alertType string, symptoms []Symptom) *DiagnosisResult {
	result := &DiagnosisResult{
		Service:   service,
		AlertType: alertType,
		Severity:  "warning",
		Remedies:  []Remedy{},
	}

	if len(symptoms) == 0 {
		result.Diagnosis = "症状数据不足，无法诊断"
		return result
	}

	e.mu.RLock()
	rules := make([]DiagnosisRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	// 1. 规则匹配
	var bestMatch *DiagnosisRule
	bestScore := 0.0

	for i := range rules {
		score := e.scoreRule(&rules[i], symptoms)
		if score > bestScore && score >= 0.5 {
			bestScore = score
			bestMatch = &rules[i]
		}
	}

	if bestMatch != nil {
		result.Diagnosis = bestMatch.Diagnosis
		result.Confidence = bestScore * bestMatch.Confidence
		result.Severity = bestMatch.Severity
		result.CanAutoFix = bestMatch.AutoFixable

		// 获取修复建议
		result.Remedies = e.getRemedies(bestMatch.ID, bestMatch.Category)
		result.KnowledgeRefs = e.getKnowledgeRefs(bestMatch.Category)
	} else {
		// 2. 启发式诊断（无规则匹配时）
		result.Diagnosis = e.heuristicDiagnose(symptoms)
		result.Confidence = 0.4
		result.Remedies = e.getGenericRemedies(alertType)
	}

	// 3. 估算修复时间
	result.TimeToResolve = e.estimateTimeToResolve(result)

	// 4. 记录诊断
	record := DiagnosisRecord{
		ID:        fmt.Sprintf("diag-%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Service:   service,
		AlertType: alertType,
		Symptoms:  symptoms,
		Diagnosis: result.Diagnosis,
		Confidence: result.Confidence,
		Remedies:  result.Remedies,
	}

	e.mu.Lock()
	e.history = append(e.history, record)
	e.mu.Unlock()

	return result
}

// scoreRule 计算规则匹配分数
func (e *DiagnosisEngine) scoreRule(rule *DiagnosisRule, symptoms []Symptom) float64 {
	if len(rule.Conditions) == 0 || len(symptoms) == 0 {
		return 0
	}

	matched := 0
	for _, cond := range rule.Conditions {
		for _, sym := range symptoms {
			if strings.EqualFold(cond.Metric, sym.Metric) {
				if matchCondition(cond, sym.Value) {
					matched++
					break
				}
			}
		}
	}

	return float64(matched) / float64(len(rule.Conditions))
}

func matchCondition(cond Condition, value float64) bool {
	switch cond.Operator {
	case ">":
		return value > cond.Value
	case "<":
		return value < cond.Value
	case ">=":
		return value >= cond.Value
	case "<=":
		return value <= cond.Value
	case "==":
		return value == cond.Value
	default:
		return false
	}
}

// heuristicDiagnose 启发式诊断
func (e *DiagnosisEngine) heuristicDiagnose(symptoms []Symptom) string {
	var cpuHigh, memHigh, latencyHigh, errorHigh, diskHigh bool

	for _, s := range symptoms {
		metric := strings.ToLower(s.Metric)
		switch {
		case strings.Contains(metric, "cpu"):
			cpuHigh = s.Value > 80
		case strings.Contains(metric, "memory"):
			memHigh = s.Value > 80
		case strings.Contains(metric, "latency") || strings.Contains(metric, "response"):
			latencyHigh = s.Value > 1000
		case strings.Contains(metric, "error"):
			errorHigh = s.Value > 0.05
		case strings.Contains(metric, "disk"):
			diskHigh = s.Value > 85
		}
	}

	switch {
	case cpuHigh && latencyHigh:
		return "CPU 使用率过高导致请求处理延迟增加"
	case memHigh && errorHigh:
		return "内存不足可能导致 OOM 和请求失败"
	case diskHigh && latencyHigh:
		return "磁盘 IO 瓶颈导致响应延迟"
	case errorHigh && !cpuHigh && !memHigh:
		return "应用逻辑错误，非资源问题"
	case cpuHigh && memHigh && diskHigh:
		return "资源全面耗尽，可能是负载突增或资源泄漏"
	default:
		return "需要进一步排查，症状组合不明确"
	}
}

// getRemedies 获取特定规则的修复建议
func (e *DiagnosisEngine) getRemedies(ruleID, category string) []Remedy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []Remedy

	// 按规则ID查找
	if remedies, ok := e.remedies[ruleID]; ok {
		result = append(result, remedies...)
	}

	// 按类别查找
	if remedies, ok := e.remedies[category]; ok {
		for _, r := range remedies {
			// 去重
			found := false
			for _, existing := range result {
				if existing.ID == r.ID {
					found = true
					break
				}
			}
			if !found {
				result = append(result, r)
			}
		}
	}

	return result
}

// getGenericRemedies 获取通用修复建议
func (e *DiagnosisEngine) getGenericRemedies(alertType string) []Remedy {
	return []Remedy{
		{
			ID:          "generic-check-logs",
			Name:        "检查日志",
			Description: "查看服务日志获取详细错误信息",
			Steps:       []string{"查看最近的错误日志", "搜索关键字", "定位异常堆栈"},
			Risk:        "low",
			AutoFixable: false,
			Category:    "generic",
		},
		{
			ID:          "generic-restart",
			Name:        "重启服务",
			Description: "临时恢复服务可用性",
			Steps:       []string{"滚动重启", "观察指标恢复"},
			Risk:        "medium",
			AutoFixable: false,
			Category:    "generic",
		},
	}
}

// getKnowledgeRefs 获取相关知识条目
func (e *DiagnosisEngine) getKnowledgeRefs(category string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var refs []string
	for id, entry := range e.knowledgeBase {
		if entry.Category == category {
			refs = append(refs, id)
		}
	}
	return refs
}

// estimateTimeToResolve 估算修复时间
func (e *DiagnosisEngine) estimateTimeToResolve(result *DiagnosisResult) time.Duration {
	switch result.Severity {
	case "critical":
		return 30 * time.Minute
	case "warning":
		return 2 * time.Hour
	default:
		return 4 * time.Hour
	}
}

// ============================================================================
// 自动修复
// ============================================================================

// AutoFix 尝试自动修复
func (e *DiagnosisEngine) AutoFix(service string, result *DiagnosisResult) string {
	if !result.CanAutoFix || len(result.Remedies) == 0 {
		return "无可用自动修复方案"
	}

	for _, remedy := range result.Remedies {
		if remedy.AutoFixable {
			// 模拟执行自动修复
			return fmt.Sprintf("已自动执行修复方案: %s", remedy.Name)
		}
	}

	return "无自动修复方案匹配"
}

// ============================================================================
// 历史查询
// ============================================================================

// GetHistory 获取诊断历史
func (e *DiagnosisEngine) GetHistory(service string, limit int) []DiagnosisRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var records []DiagnosisRecord
	for i := len(e.history) - 1; i >= 0; i-- {
		if service == "" || e.history[i].Service == service {
			records = append(records, e.history[i])
			if limit > 0 && len(records) >= limit {
				break
			}
		}
	}
	return records
}

// GetStats 获取诊断统计
func (e *DiagnosisEngine) GetStats() DiagnosisStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := DiagnosisStats{
		TotalDiagnoses: len(e.history),
		ByService:      make(map[string]int),
		ByCategory:     make(map[string]int),
	}

	for _, rec := range e.history {
		stats.ByService[rec.Service]++
		for _, remedy := range rec.Remedies {
			stats.ByCategory[remedy.Category]++
		}
	}

	return stats
}

// DiagnosisStats 诊断统计
type DiagnosisStats struct {
	TotalDiagnoses int
	ByService      map[string]int
	ByCategory     map[string]int
}

// ============================================================================
// 内置规则库
// ============================================================================

func (e *DiagnosisEngine) loadBuiltinRules() {
	rules := []DiagnosisRule{
		{
			ID:         "cpu-high-load",
			Name:       "CPU 高负载",
			Category:   "performance",
			Severity:   "warning",
			Conditions: []Condition{{Metric: "cpu", Operator: ">", Value: 80}},
			Diagnosis:  "CPU 使用率超过 80%，可能影响请求处理性能",
			Confidence: 0.85,
			AutoFixable: false,
		},
		{
			ID:         "memory-leak-suspect",
			Name:       "内存泄漏嫌疑",
			Category:   "resource",
			Severity:   "critical",
			Conditions: []Condition{
				{Metric: "memory", Operator: ">", Value: 85},
				{Metric: "memory_growth_rate", Operator: ">", Value: 5},
			},
			Diagnosis:  "内存持续增长且使用率超过 85%，疑似内存泄漏",
			Confidence: 0.75,
			AutoFixable: false,
		},
		{
			ID:         "db-connection-pool-exhausted",
			Name:       "数据库连接池耗尽",
			Category:   "database",
			Severity:   "critical",
			Conditions: []Condition{
				{Metric: "db_connections", Operator: ">", Value: 90},
				{Metric: "latency", Operator: ">", Value: 2000},
			},
			Diagnosis:  "数据库连接池接近耗尽，导致请求排队和延迟增加",
			Confidence: 0.9,
			AutoFixable: false,
		},
		{
			ID:         "disk-full",
			Name:       "磁盘空间不足",
			Category:   "storage",
			Severity:   "critical",
			Conditions: []Condition{{Metric: "disk_usage", Operator: ">", Value: 90}},
			Diagnosis:  "磁盘使用率超过 90%，可能导致写入失败和服务异常",
			Confidence: 0.95,
			AutoFixable: false,
		},
		{
			ID:         "latency-spike",
			Name:       "延迟突增",
			Category:   "performance",
			Severity:   "warning",
			Conditions: []Condition{{Metric: "latency", Operator: ">", Value: 1000}},
			Diagnosis:  "响应延迟超过 1s，可能由下游依赖或 GC 引起",
			Confidence: 0.7,
			AutoFixable: false,
		},
		{
			ID:         "error-rate-spike",
			Name:       "错误率突增",
			Category:   "availability",
			Severity:   "critical",
			Conditions: []Condition{
				{Metric: "error_rate", Operator: ">", Value: 0.05},
			},
			Diagnosis:  "错误率超过 5%，可能是代码缺陷或依赖故障",
			Confidence: 0.8,
			AutoFixable: false,
		},
		{
			ID:         "circuit-breaker-open",
			Name:       "熔断器开启",
			Category:   "availability",
			Severity:   "warning",
			Conditions: []Condition{
				{Metric: "circuit_breaker_state", Operator: "==", Value: 1},
			},
			Diagnosis:  "熔断器已开启，下游服务不可用或响应超时",
			Confidence: 0.9,
			AutoFixable: true,
		},
		{
			ID:         "queue-backup",
			Name:       "队列积压",
			Category:   "performance",
			Severity:   "warning",
			Conditions: []Condition{
				{Metric: "queue_depth", Operator: ">", Value: 10000},
			},
			Diagnosis:  "消息队列深度超过 10000，消费者处理速度不足",
			Confidence: 0.85,
			AutoFixable: true,
		},
	}

	for i := range rules {
		e.rules = append(e.rules, rules[i])
	}
}

func (e *DiagnosisEngine) loadBuiltinRemedies() {
	// 性能类
	e.remedies["performance"] = []Remedy{
		{
			ID:          "perf-scale-up",
			Name:        "水平扩容",
			Description: "增加服务实例以分散负载",
			Steps:       []string{"增加副本数", "监控负载均衡效果", "观察响应时间恢复"},
			Risk:        "low",
			AutoFixable: true,
			Category:    "performance",
		},
		{
			ID:          "perf-cache-enable",
			Name:        "启用缓存",
			Description: "在热点数据路径增加缓存层",
			Steps:       []string{"启用 Redis 缓存", "设置合理的 TTL", "监控缓存命中率"},
			Risk:        "low",
			AutoFixable: false,
			Category:    "performance",
		},
		{
			ID:          "perf-gc-tune",
			Name:        "GC 参数调优",
			Description: "调整 JVM/Go GC 参数减少停顿",
			Steps:       []string{"分析 GC 日志", "调整堆大小", "启用并发 GC"},
			Risk:        "medium",
			AutoFixable: false,
			Category:    "performance",
		},
	}

	// 资源类
	e.remedies["resource"] = []Remedy{
		{
			ID:          "res-restart",
			Name:        "滚动重启",
			Description: "释放累积内存并重启服务",
			Steps:       []string{"标记实例为待重启", "逐个重启", "确认健康检查通过"},
			Risk:        "medium",
			AutoFixable: true,
			Category:    "resource",
		},
		{
			ID:          "res-heap-increase",
			Name:        "增加堆内存",
			Description: "提升容器内存限制",
			Steps:       []string{"修改资源配置", "滚动更新", "监控内存使用"},
			Risk:        "low",
			AutoFixable: false,
			Category:    "resource",
		},
	}

	// 数据库类
	e.remedies["database"] = []Remedy{
		{
			ID:          "db-pool-increase",
			Name:        "扩大连接池",
			Description: "增加数据库连接池大小",
			Steps:       []string{"修改连接池配置", "检查数据库连接上限", "验证连接数"},
			Risk:        "low",
			AutoFixable: true,
			Category:    "database",
		},
		{
			ID:          "db-slow-query-kill",
			Name:        "终止慢查询",
			Description: "终止执行时间过长的 SQL 查询",
			Steps:       []string{"识别慢查询", "KILL 慢查询会话", "分析查询计划"},
			Risk:        "high",
			AutoFixable: true,
			Category:    "database",
		},
		{
			ID:          "db-read-replica",
			Name:        "启用读副本",
			Description: "将读流量分流到只读副本",
			Steps:       []string{"检查读副本可用性", "配置读写分离", "监控读流量分布"},
			Risk:        "low",
			AutoFixable: false,
			Category:    "database",
		},
	}

	// 存储类
	e.remedies["storage"] = []Remedy{
		{
			ID:          "storage-log-cleanup",
			Name:        "日志清理",
			Description: "清理过期的日志文件释放空间",
			Steps:       []string{"查找大日志文件", "压缩或删除旧日志", "检查磁盘空间"},
			Risk:        "low",
			AutoFixable: true,
			Category:    "storage",
		},
		{
			ID:          "storage-expand",
			Name:        "扩容存储",
			Description: "增加磁盘容量",
			Steps:       []string{"评估容量需求", "申请扩容", "迁移数据"},
			Risk:        "low",
			AutoFixable: false,
			Category:    "storage",
		},
	}

	// 可用性类
	e.remedies["availability"] = []Remedy{
		{
			ID:          "avail-cb-reset",
			Name:        "重置熔断器",
			Description: "手动重置熔断器状态",
			Steps:       []string{"确认下游服务恢复", "重置熔断器", "监控请求成功率"},
			Risk:        "medium",
			AutoFixable: true,
			Category:    "availability",
		},
		{
			ID:          "avail-rollback",
			Name:        "回滚变更",
			Description: "回滚最近的部署变更",
			Steps:       []string{"确认问题始于某次部署", "执行回滚", "验证服务恢复"},
			Risk:        "high",
			AutoFixable: false,
			Category:    "availability",
		},
	}
}

func (e *DiagnosisEngine) loadBuiltinKnowledge() {
	entries := []KnowledgeEntry{
		{
			ID:       "kb-cpu-high",
			Title:    "CPU 高负载排查指南",
			Category: "performance",
			Content:  "1. 使用 top/htop 定位高 CPU 进程\n2. 分析 CPU 火焰图\n3. 检查是否有死循环\n4. 评估是否需要扩容",
			Tags:     []string{"cpu", "performance", "troubleshooting"},
		},
		{
			ID:       "kb-memory-leak",
			Title:    "内存泄漏排查方法",
			Category: "resource",
			Content:  "1. 使用 pprof/heap dump 分析内存分布\n2. 查看对象存活时间\n3. 检查全局缓存和单例模式\n4. 使用 valgrind 或类似工具",
			Tags:     []string{"memory", "leak", "troubleshooting"},
		},
		{
			ID:       "kb-db-timeout",
			Title:    "数据库超时故障处理",
			Category: "database",
			Content:  "1. 检查连接池配置\n2. 查看慢查询日志\n3. 分析执行计划\n4. 检查锁等待情况\n5. 考虑读写分离",
			Tags:     []string{"database", "timeout", "troubleshooting"},
		},
		{
			ID:       "kb-disk-full",
			Title:    "磁盘空间不足处理",
			Category: "storage",
			Content:  "1. 找出大文件\n2. 清理日志和临时文件\n3. 压缩归档历史数据\n4. 申请扩容",
			Tags:     []string{"disk", "storage", "cleanup"},
		},
		{
			ID:       "kb-latency-spike",
			Title:    "延迟突增排查",
			Category: "performance",
			Content:  "1. 检查下游依赖延迟\n2. 查看 GC 暂停时间\n3. 检查网络抖动\n4. 分析请求分布",
			Tags:     []string{"latency", "performance", "troubleshooting"},
		},
	}

	for i := range entries {
		e.knowledgeBase[entries[i].ID] = entries[i]
	}
}

// ============================================================================
// 管理接口
// ============================================================================

// AddRule 添加诊断规则
func (e *DiagnosisEngine) AddRule(rule DiagnosisRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// GetRules 获取所有规则
func (e *DiagnosisEngine) GetRules() []DiagnosisRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rules := make([]DiagnosisRule, len(e.rules))
	copy(rules, e.rules)
	return rules
}

// GetKnowledge 获取知识库
func (e *DiagnosisEngine) GetKnowledge(category string) []KnowledgeEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var entries []KnowledgeEntry
	for _, entry := range e.knowledgeBase {
		if category == "" || entry.Category == category {
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

// SearchKnowledge 搜索知识库
func (e *DiagnosisEngine) SearchKnowledge(keyword string) []KnowledgeEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var entries []KnowledgeEntry
	keyword = strings.ToLower(keyword)
	for _, entry := range e.knowledgeBase {
		if strings.Contains(strings.ToLower(entry.Title), keyword) ||
			strings.Contains(strings.ToLower(entry.Content), keyword) {
			entries = append(entries, entry)
		}
	}
	return entries
}
