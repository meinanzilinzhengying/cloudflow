//go:build linux

package pipeline

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ============================================================================
// 一、预处理阶段定义
// ============================================================================

// PreprocessStage 预处理阶段
type PreprocessStage string

const (
	StageFilter  PreprocessStage = "filter"  // 过滤
	StageSample  PreprocessStage = "sample"  // 采样
	StageEnrich  PreprocessStage = "enrich"  // 富化
	StageAggregate PreprocessStage = "aggregate" // 聚合
)

// Preprocessor 预处理器接口
type Preprocessor interface {
	Process(record *DataRecord) error
	Name() string
	Stage() PreprocessStage
}

// ============================================================================
// 二、过滤预处理器
// ============================================================================

// FilterRule 过滤规则
type FilterRule struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // eq, ne, gt, lt, contains, regex
	Value    interface{} `json:"value"`
}

// Match 检查记录是否匹配规则
func (fr *FilterRule) Match(record *DataRecord) bool {
	if record.Meta == nil {
		return false
	}
	
	var fieldValue string
	switch fr.Field {
	case "src_ip":
		fieldValue = record.Meta.SrcIP
	case "dst_ip":
		fieldValue = record.Meta.DstIP
	case "namespace":
		fieldValue = record.Meta.Namespace
	case "service":
		fieldValue = record.Meta.Service
	case "pod":
		fieldValue = record.Meta.Pod
	case "node":
		fieldValue = record.Meta.Node
	case "protocol":
		fieldValue = fmt.Sprintf("%d", record.Meta.Protocol)
	case "l7_protocol":
		fieldValue = fmt.Sprintf("%d", record.Meta.L7Protocol)
	case "tenant_id":
		fieldValue = record.TenantID
	case "probe_id":
		fieldValue = record.ProbeID
	default:
		if record.Meta.Tags != nil {
			fieldValue = record.Meta.Tags[fr.Field]
		}
	}
	
	return matchValue(fieldValue, fr.Operator, fr.Value)
}

func matchValue(fieldValue, operator string, expected interface{}) bool {
	expectedStr := fmt.Sprintf("%v", expected)
	
	switch operator {
	case "eq":
		return fieldValue == expectedStr
	case "ne":
		return fieldValue != expectedStr
	case "contains":
		return contains(fieldValue, expectedStr)
	case "gt", "lt":
		// 简化的数值比较
		return false
	default:
		return false
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// FilterPreprocessor 过滤预处理器
type FilterPreprocessor struct {
	rules     []*FilterRule
	dropEmpty bool // 是否丢弃空元数据记录
}

// NewFilterPreprocessor 创建过滤预处理器
func NewFilterPreprocessor(rules []*FilterRule) *FilterPreprocessor {
	return &FilterPreprocessor{
		rules:     rules,
		dropEmpty: true,
	}
}

// Name 返回名称
func (fp *FilterPreprocessor) Name() string {
	return "filter"
}

// Stage 返回阶段
func (fp *FilterPreprocessor) Stage() PreprocessStage {
	return StageFilter
}

// Process 处理记录
func (fp *FilterPreprocessor) Process(record *DataRecord) error {
	// 检查空元数据
	if fp.dropEmpty && record.Meta == nil {
		record.Dropped = true
		record.DropReason = "empty metadata"
		return nil
	}
	
	// 检查规则
	for _, rule := range fp.rules {
		if rule.Match(record) {
			record.Dropped = true
			record.DropReason = fmt.Sprintf("matched filter rule: %s %s %v", rule.Field, rule.Operator, rule.Value)
			return nil
		}
	}
	
	return nil
}

// AddRule 添加规则
func (fp *FilterPreprocessor) AddRule(rule *FilterRule) {
	fp.rules = append(fp.rules, rule)
}

// ============================================================================
// 三、采样预处理器
// ============================================================================

// SamplePreprocessor 采样预处理器
type SamplePreprocessor struct {
	rate       float64
	strategy   SampleStrategy
	seed       int64
	count      int64
	mu         sync.RWMutex
}

// SampleStrategy 采样策略
type SampleStrategy string

const (
	SampleStrategyRandom    SampleStrategy = "random"    // 随机采样
	SampleStrategyConsistent SampleStrategy = "consistent" // 一致性哈希采样
	SampleStrategyCount      SampleStrategy = "count"      // 计数采样（每N取1）
)

// NewSamplePreprocessor 创建采样预处理器
func NewSamplePreprocessor(rate float64, strategy SampleStrategy) *SamplePreprocessor {
	return &SamplePreprocessor{
		rate:     rate,
		strategy: strategy,
		seed:     time.Now().UnixNano(),
	}
}

// Name 返回名称
func (sp *SamplePreprocessor) Name() string {
	return "sample"
}

// Stage 返回阶段
func (sp *SamplePreprocessor) Stage() PreprocessStage {
	return StageSample
}

// Process 处理记录
func (sp *SamplePreprocessor) Process(record *DataRecord) error {
	if sp.rate >= 1.0 {
		return nil // 全量保留
	}
	if sp.rate <= 0 {
		record.Dropped = true
		record.DropReason = "sample rate=0"
		return nil
	}
	
	switch sp.strategy {
	case SampleStrategyRandom:
		sp.processRandom(record)
	case SampleStrategyConsistent:
		sp.processConsistent(record)
	case SampleStrategyCount:
		sp.processCount(record)
	default:
		sp.processRandom(record)
	}
	
	return nil
}

func (sp *SamplePreprocessor) processRandom(record *DataRecord) {
	if rand.Float64() >= sp.rate {
		record.Dropped = true
		record.DropReason = fmt.Sprintf("random sample dropped (rate=%.2f)", sp.rate)
	}
}

func (sp *SamplePreprocessor) processConsistent(record *DataRecord) {
	// 使用记录ID的一致性哈希，确保相同ID始终被采样或丢弃
	hash := hashString(record.ID + fmt.Sprintf("%d", sp.seed))
	if float64(hash%10000)/10000.0 >= sp.rate {
		record.Dropped = true
		record.DropReason = fmt.Sprintf("consistent sample dropped (rate=%.2f)", sp.rate)
	}
}

func (sp *SamplePreprocessor) processCount(record *DataRecord) {
	sp.mu.Lock()
	sp.count++
	count := sp.count
	sp.mu.Unlock()
	
	interval := int64(1.0 / sp.rate)
	if interval <= 0 {
		interval = 1
	}
	if count%interval != 0 {
		record.Dropped = true
		record.DropReason = fmt.Sprintf("count sample dropped (interval=%d)", interval)
	}
}

func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// ============================================================================
// 四、富化预处理器
// ============================================================================

// EnrichPreprocessor 富化预处理器
type EnrichPreprocessor struct {
	enrichers []func(*DataRecord) error
}

// NewEnrichPreprocessor 创建富化预处理器
func NewEnrichPreprocessor() *EnrichPreprocessor {
	return &EnrichPreprocessor{
		enrichers: make([]func(*DataRecord) error, 0),
	}
}

// Name 返回名称
func (ep *EnrichPreprocessor) Name() string {
	return "enrich"
}

// Stage 返回阶段
func (ep *EnrichPreprocessor) Stage() PreprocessStage {
	return StageEnrich
}

// Process 处理记录
func (ep *EnrichPreprocessor) Process(record *DataRecord) error {
	if record.Meta == nil {
		record.Meta = &DataMeta{}
	}
	
	for _, enricher := range ep.enrichers {
		if err := enricher(record); err != nil {
			return err
		}
	}
	
	record.Processed = true
	return nil
}

// AddEnricher 添加富化器
func (ep *EnrichPreprocessor) AddEnricher(enricher func(*DataRecord) error) {
	ep.enrichers = append(ep.enrichers, enricher)
}

// AddTimeEnricher 添加时间富化器
func AddTimeEnricher() func(*DataRecord) error {
	return func(record *DataRecord) error {
		if record.Meta == nil {
			record.Meta = &DataMeta{}
		}
		if record.Meta.Tags == nil {
			record.Meta.Tags = make(map[string]string)
		}
		record.Meta.Tags["hour"] = time.Now().Format("15")
		record.Meta.Tags["day_of_week"] = fmt.Sprintf("%d", time.Now().Weekday())
		return nil
	}
}

// AddGeoEnricher 添加地理位置富化器（简化）
func AddGeoEnricher() func(*DataRecord) error {
	return func(record *DataRecord) error {
		if record.Meta == nil || record.Meta.SrcIP == "" {
			return nil
		}
		if record.Meta.Tags == nil {
			record.Meta.Tags = make(map[string]string)
		}
		// 简化的 IP 段判断
		if isPrivateIP(record.Meta.SrcIP) {
			record.Meta.Tags["src_geo"] = "private"
		} else {
			record.Meta.Tags["src_geo"] = "public"
		}
		return nil
	}
}

func isPrivateIP(ip string) bool {
	// 简化判断：以 10. 或 192.168. 或 172. 开头
	return len(ip) > 3 && (ip[:3] == "10." || ip[:7] == "192.168" || (len(ip) > 6 && ip[:6] == "172.16"))
}

// ============================================================================
// 五、预处理管道
// ============================================================================

// PreprocessPipeline 预处理管道
type PreprocessPipeline struct {
	preprocessors []Preprocessor
	mu            sync.RWMutex
}

// NewPreprocessPipeline 创建预处理管道
func NewPreprocessPipeline() *PreprocessPipeline {
	return &PreprocessPipeline{
		preprocessors: make([]Preprocessor, 0),
	}
}

// Add 添加预处理器
func (pp *PreprocessPipeline) Add(p Preprocessor) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	pp.preprocessors = append(pp.preprocessors, p)
}

// Process 处理单条记录
func (pp *PreprocessPipeline) Process(record *DataRecord) error {
	pp.mu.RLock()
	processors := make([]Preprocessor, len(pp.preprocessors))
	copy(processors, pp.preprocessors)
	pp.mu.RUnlock()
	
	for _, processor := range processors {
		if record.Dropped {
			return nil // 已丢弃，不再处理
		}
		if err := processor.Process(record); err != nil {
			return fmt.Errorf("preprocessor %s failed: %w", processor.Name(), err)
		}
	}
	
	return nil
}

// ProcessBatch 处理批次
func (pp *PreprocessPipeline) ProcessBatch(batch *DataBatch) error {
	for _, record := range batch.Records {
		if err := pp.Process(record); err != nil {
			return err
		}
	}
	return nil
}

// GetStats 获取统计信息
func (pp *PreprocessPipeline) GetStats() map[string]interface{} {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	
	stages := make(map[string]int)
	for _, p := range pp.preprocessors {
		stages[string(p.Stage())]++
	}
	
	return map[string]interface{}{
		"processor_count": len(pp.preprocessors),
		"stages":          stages,
	}
}

// Clear 清空所有处理器
func (pp *PreprocessPipeline) Clear() {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	pp.preprocessors = make([]Preprocessor, 0)
}
