// Package profiler 提供 Java 堆外内存检测功能
// 本文件实现泄漏检测引擎，识别未释放的内存块并生成分析报告
package profiler

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// ==================== 泄漏检测引擎 ====================

// LeakDetectorConfig 泄漏检测配置
type LeakDetectorConfig struct {
	MinLeakAge     time.Duration // 最小泄漏年龄（分配后多长时间未释放视为可疑），默认 30s
	MinBlockSize   int64         // 最小关注块大小(字节)，默认 1024 (1KB)
	MaxReportCount int           // 最大报告泄漏数量，默认 100
	GroupByStack   bool          // 是否按调用栈分组，默认 true
	SortBy         string        // 排序方式: "size" | "age" | "count"，默认 "size"
}

// DefaultLeakDetectorConfig 返回默认配置
func DefaultLeakDetectorConfig() *LeakDetectorConfig {
	return &LeakDetectorConfig{
		MinLeakAge:     30 * time.Second,
		MinBlockSize:   1024,
		MaxReportCount: 100,
		GroupByStack:   true,
		SortBy:         "size",
	}
}

// LeakInfo 泄漏信息
type LeakInfo struct {
	Address      uint64      `json:"address"`       // 内存地址
	Size         int64       `json:"size"`          // 大小(字节)
	Source       AllocSource `json:"source"`        // 分配来源
	AllocTime    time.Time   `json:"alloc_time"`    // 分配时间
	Age          time.Duration `json:"age"`          // 已存活时间
	JavaStack    []string    `json:"java_stack"`    // Java 调用栈
	NativeStack  []string    `json:"native_stack"`  // 原生调用栈
	JNIClass     string      `json:"jni_class"`     // JNI 类名
	JNIMethod    string      `json:"jni_method"`    // JNI 方法名
	Severity     string      `json:"severity"`      // 严重程度: low/medium/high/critical
	Score        float64     `json:"score"`         // 泄漏评分 (0-100)
}

// LeakGroup 按调用栈分组的泄漏信息
type LeakGroup struct {
	StackKey     string      `json:"stack_key"`     // 栈签名（用于分组）
	JavaStack    []string    `json:"java_stack"`    // 代表性 Java 调用栈
	Source       AllocSource `json:"source"`        // 分配来源
	Count        int         `json:"count"`         // 泄漏块数
	TotalSize    int64       `json:"total_size"`    // 总泄漏大小(字节)
	MinSize      int64       `json:"min_size"`      // 最小块大小
	MaxSize      int64       `json:"max_size"`      // 最大块大小
	AvgSize      float64     `json:"avg_size"`      // 平均块大小
	OldestAlloc  time.Time   `json:"oldest_alloc"`  // 最早的分配时间
	NewestAlloc  time.Time   `json:"newest_alloc"`  // 最新的分配时间
	Severity     string      `json:"severity"`      // 严重程度
	Score        float64     `json:"score"`         // 泄漏评分
}

// LeakSummary 泄漏检测摘要
type LeakSummary struct {
	Timestamp       time.Time    `json:"timestamp"`
	PID             uint32       `json:"pid"`
	TotalLeakCount  int          `json:"total_leak_count"`
	TotalLeakSize   int64        `json:"total_leak_size"`
	CriticalCount   int          `json:"critical_count"`
	HighCount       int          `json:"high_count"`
	MediumCount     int          `json:"medium_count"`
	LowCount        int          `json:"low_count"`
	TopLeakGroups   []LeakGroup  `json:"top_leak_groups"`
	SourceStats     map[AllocSource]*SourceLeakStat `json:"source_stats"`
}

// SourceLeakStat 按来源的泄漏统计
type SourceLeakStat struct {
	Count     int   `json:"count"`
	TotalSize int64 `json:"total_size"`
	Percentage float64 `json:"percentage"`
}

// ==================== 泄漏检测引擎 ====================

// LeakDetector 泄漏检测引擎
type LeakDetector struct {
	config *LeakDetectorConfig
}

// NewLeakDetector 创建泄漏检测引擎
func NewLeakDetector(cfg *LeakDetectorConfig) *LeakDetector {
	if cfg == nil {
		cfg = DefaultLeakDetectorConfig()
	}
	return &LeakDetector{config: cfg}
}

// Detect 执行泄漏检测
func (d *LeakDetector) Detect(blocks []*MemoryBlock, now time.Time) *LeakSummary {
	summary := &LeakSummary{
		Timestamp:   now,
		SourceStats: make(map[AllocSource]*SourceLeakStat),
	}

	// 1. 筛选可疑泄漏块
	var leaks []LeakInfo
	for _, block := range blocks {
		if block.Released {
			continue
		}
		if block.Size < d.config.MinBlockSize {
			continue
		}

		age := now.Sub(block.AllocTime)
		if age < d.config.MinLeakAge {
			continue
		}

		leak := LeakInfo{
			Address:     block.Address,
			Size:        block.Size,
			Source:      block.Source,
			AllocTime:   block.AllocTime,
			Age:         age,
			JavaStack:   block.JavaStack,
			NativeStack: block.NativeStack,
			JNIClass:    block.JNIClass,
			JNIMethod:   block.JNIMethod,
		}

		// 计算严重程度
		leak.Severity, leak.Score = d.calculateSeverity(block.Size, age)

		leaks = append(leaks, leak)

		// 按来源统计
		if _, ok := summary.SourceStats[block.Source]; !ok {
			summary.SourceStats[block.Source] = &SourceLeakStat{}
		}
		stat := summary.SourceStats[block.Source]
		stat.Count++
		stat.TotalSize += block.Size

		// 按严重程度计数
		switch leak.Severity {
		case "critical":
			summary.CriticalCount++
		case "high":
			summary.HighCount++
		case "medium":
			summary.MediumCount++
		case "low":
			summary.LowCount++
		}
	}

	summary.TotalLeakCount = len(leaks)

	// 计算总泄漏大小
	for _, leak := range leaks {
		summary.TotalLeakSize += leak.Size
	}

	// 计算百分比
	if summary.TotalLeakSize > 0 {
		for _, stat := range summary.SourceStats {
			stat.Percentage = float64(stat.TotalSize) * 100.0 / float64(summary.TotalLeakSize)
		}
	}

	// 2. 按调用栈分组
	if d.config.GroupByStack {
		summary.TopLeakGroups = d.groupLeaks(leaks)
	} else {
		// 不分组，直接排序
		summary.TopLeakGroups = d.convertToGroups(leaks)
	}

	// 3. 排序
	d.sortGroups(summary.TopLeakGroups)

	// 4. 限制数量
	if d.config.MaxReportCount > 0 && len(summary.TopLeakGroups) > d.config.MaxReportCount {
		summary.TopLeakGroups = summary.TopLeakGroups[:d.config.MaxReportCount]
	}

	return summary
}

// calculateSeverity 计算泄漏严重程度
func (d *LeakDetector) calculateSeverity(size int64, age time.Duration) (string, float64) {
	// 评分维度: 大小权重 + 年龄权重
	sizeScore := float64(size) / (1024 * 1024) // 每 MB 得 1 分
	ageScore := float64(age.Minutes()) / 10.0    // 每 10 分钟得 1 分

	score := sizeScore + ageScore

	// 限制最大分数
	if score > 100 {
		score = 100
	}

	// 确定严重程度
	var severity string
	switch {
	case score >= 50:
		severity = "critical"
	case score >= 20:
		severity = "high"
	case score >= 5:
		severity = "medium"
	default:
		severity = "low"
	}

	return severity, score
}

// groupLeaks 按调用栈分组泄漏
func (d *LeakDetector) groupLeaks(leaks []LeakInfo) []LeakGroup {
	groupMap := make(map[string]*LeakGroup)

	for _, leak := range leaks {
		key := d.stackToKey(leak.JavaStack)

		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &LeakGroup{
				StackKey:  key,
				JavaStack: leak.JavaStack,
				Source:    leak.Source,
				MinSize:   leak.Size,
				MaxSize:   leak.Size,
				OldestAlloc: leak.AllocTime,
				NewestAlloc: leak.AllocTime,
			}
		}

		group := groupMap[key]
		group.Count++
		group.TotalSize += leak.Size
		if leak.Size < group.MinSize {
			group.MinSize = leak.Size
		}
		if leak.Size > group.MaxSize {
			group.MaxSize = leak.Size
		}
		if leak.AllocTime.Before(group.OldestAlloc) {
			group.OldestAlloc = leak.AllocTime
		}
		if leak.AllocTime.After(group.NewestAlloc) {
			group.NewestAlloc = leak.AllocTime
		}
		if leak.Score > group.Score {
			group.Score = leak.Score
		}
		if leak.Severity == "critical" || group.Severity == "" {
			group.Severity = leak.Severity
		}
	}

	groups := make([]LeakGroup, 0, len(groupMap))
	for _, g := range groupMap {
		g.AvgSize = float64(g.TotalSize) / float64(g.Count)
		groups = append(groups, *g)
	}

	return groups
}

// stackToKey 将调用栈转换为分组 key
func (d *LeakDetector) stackToKey(stack []string) string {
	if len(stack) == 0 {
		return "unknown"
	}
	// 取前 5 个栈帧作为 key
	n := len(stack)
	if n > 5 {
		n = 5
	}
	return strings.Join(stack[:n], ";")
}

// convertToGroups 将泄漏列表转换为单块组
func (d *LeakDetector) convertToGroups(leaks []LeakInfo) []LeakGroup {
	groups := make([]LeakGroup, 0, len(leaks))
	for _, leak := range leaks {
		groups = append(groups, LeakGroup{
			StackKey:    fmt.Sprintf("%d", leak.Address),
			JavaStack:   leak.JavaStack,
			Source:      leak.Source,
			Count:       1,
			TotalSize:   leak.Size,
			MinSize:     leak.Size,
			MaxSize:     leak.Size,
			AvgSize:     float64(leak.Size),
			OldestAlloc: leak.AllocTime,
			NewestAlloc: leak.AllocTime,
			Severity:    leak.Severity,
			Score:       leak.Score,
		})
	}
	return groups
}

// sortGroups 排序泄漏组
func (d *LeakDetector) sortGroups(groups []LeakGroup) {
	switch d.config.SortBy {
	case "size":
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].TotalSize > groups[j].TotalSize
		})
	case "age":
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].OldestAlloc.Before(groups[j].OldestAlloc)
		})
	case "count":
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].Count > groups[j].Count
		})
	default:
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].TotalSize > groups[j].TotalSize
		})
	}
}

// ==================== 分析报告生成器 ====================

// ReportConfig 报告配置
type ReportConfig struct {
	IncludeDetails   bool // 是否包含每个泄漏块的详细信息
	IncludeStack     bool // 是否包含调用栈
	IncludeStats     bool // 是否包含统计信息
	IncludeRecommend bool // 是否包含优化建议
	MaxGroups        int  // 最大显示组数
}

// DefaultReportConfig 返回默认报告配置
func DefaultReportConfig() *ReportConfig {
	return &ReportConfig{
		IncludeDetails:   true,
		IncludeStack:     true,
		IncludeStats:     true,
		IncludeRecommend: true,
		MaxGroups:        20,
	}
}

// GenerateReport 生成泄漏分析报告
func GenerateReport(summary *LeakSummary, trackerStats NativeMemStats, javaInfo map[string]string, cfg *ReportConfig) string {
	if cfg == nil {
		cfg = DefaultReportConfig()
	}

	var buf bytes.Buffer

	w := &reportWriter{w: &buf, cfg: cfg}
	w.writeHeader(summary, javaInfo)
	w.writeOverview(summary, trackerStats)
	w.writeSourceBreakdown(summary)
	w.writeLeakGroups(summary)
	if cfg.IncludeRecommend {
		w.writeRecommendations(summary)
	}
	w.writeFooter()

	return buf.String()
}

// reportWriter 报告写入器
type reportWriter struct {
	w   io.Writer
	cfg *ReportConfig
}

func (r *reportWriter) writeLine(format string, args ...interface{}) {
	fmt.Fprintf(r.w, format+"\n", args...)
}

func (r *reportWriter) writeHeader(summary *LeakSummary, javaInfo map[string]string) {
	r.writeLine("=" * 70)
	r.writeLine("         Java 堆外内存泄漏分析报告")
	r.writeLine("=" * 70)
	r.writeLine("")
	r.writeLine("生成时间: %s", summary.Timestamp.Format("2006-01-02 15:04:05"))

	if javaInfo != nil {
		r.writeLine("Java 版本: %s", javaInfo["java_version"])
		r.writeLine("JVM 参数: %s", javaInfo["cmdline"])
		if javaInfo["max_direct_memory"] != "" {
			r.writeLine("MaxDirectMemorySize: %s", javaInfo["max_direct_memory"])
		}
	}

	r.writeLine("")
}

func (r *reportWriter) writeOverview(summary *LeakSummary, stats NativeMemStats) {
	r.writeLine("-" * 70)
	r.writeLine("【概览】")
	r.writeLine("-" * 70)
	r.writeLine("")
	r.writeLine("  堆外内存统计:")
	r.writeLine("    总分配次数:   %d", stats.TotalAllocs)
	r.writeLine("    总释放次数:   %d", stats.TotalFrees)
	r.writeLine("    总分配大小:   %s", formatBytes(stats.TotalAllocSize))
	r.writeLine("    当前活跃大小: %s", formatBytes(stats.CurrentSize))
	r.writeLine("    峰值大小:     %s", formatBytes(stats.PeakSize))
	r.writeLine("")
	r.writeLine("  泄漏检测:")
	r.writeLine("    可疑泄漏块数: %d", summary.TotalLeakCount)
	r.writeLine("    泄漏总大小:   %s", formatBytes(summary.TotalLeakSize))
	r.writeLine("    严重程度分布:")
	r.writeLine("      critical: %d", summary.CriticalCount)
	r.writeLine("      high:     %d", summary.HighCount)
	r.writeLine("      medium:   %d", summary.MediumCount)
	r.writeLine("      low:      %d", summary.LowCount)
	r.writeLine("")
}

func (r *reportWriter) writeSourceBreakdown(summary *LeakSummary) {
	r.writeLine("-" * 70)
	r.writeLine("【按来源分类】")
	r.writeLine("-" * 70)
	r.writeLine("")

	// 按大小排序
	type sourceEntry struct {
		source AllocSource
		stat   *SourceLeakStat
	}
	var entries []sourceEntry
	for source, stat := range summary.SourceStats {
		entries = append(entries, sourceEntry{source, stat})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stat.TotalSize > entries[j].stat.TotalSize
	})

	for _, entry := range entries {
		r.writeLine("  %-25s %8d 块  %12s  (%.1f%%)",
			entry.source,
			entry.stat.Count,
			formatBytes(entry.stat.TotalSize),
			entry.stat.Percentage)
	}
	r.writeLine("")
}

func (r *reportWriter) writeLeakGroups(summary *LeakSummary) {
	r.writeLine("-" * 70)
	r.writeLine("【泄漏详情 (Top %d)】", r.cfg.MaxGroups)
	r.writeLine("-" * 70)
	r.writeLine("")

	maxGroups := r.cfg.MaxGroups
	if maxGroups > len(summary.TopLeakGroups) {
		maxGroups = len(summary.TopLeakGroups)
	}

	for i := 0; i < maxGroups; i++ {
		group := summary.TopLeakGroups[i]
		r.writeLeakGroup(i+1, group)
	}
}

func (r *reportWriter) writeLeakGroup(rank int, group LeakGroup) {
	r.writeLine("  #%d [%s] 泄漏评分: %.1f", rank, strings.ToUpper(group.Severity), group.Score)
	r.writeLine("    来源: %s", group.Source)
	r.writeLine("    泄漏块数: %d, 总大小: %s (最小: %s, 最大: %s, 平均: %s)",
		group.Count,
		formatBytes(group.TotalSize),
		formatBytes(group.MinSize),
		formatBytes(group.MaxSize),
		formatBytes(int64(group.AvgSize)))
	r.writeLine("    最早分配: %s", group.OldestAlloc.Format("2006-01-02 15:04:05"))
	r.writeLine("    最近分配: %s", group.NewestAlloc.Format("2006-01-02 15:04:05"))

	if r.cfg.IncludeStack && len(group.JavaStack) > 0 {
		r.writeLine("    调用栈:")
		for _, frame := range group.JavaStack {
			r.writeLine("      at %s", frame)
		}
	}
	r.writeLine("")
}

func (r *reportWriter) writeRecommendations(summary *LeakSummary) {
	r.writeLine("-" * 70)
	r.writeLine("【优化建议】")
	r.writeLine("-" * 70)
	r.writeLine("")

	hasDirectBuffer := false
	hasUnsafe := false
	hasJNI := false

	for source, stat := range summary.SourceStats {
		switch source {
		case AllocSourceDirectByteBuffer:
			hasDirectBuffer = true
			if stat.Percentage > 30 {
				r.writeLine("  1. DirectByteBuffer 泄漏严重 (%.1f%%):", stat.Percentage)
				r.writeLine("     - 使用 -XX:MaxDirectMemorySize 限制堆外内存上限")
				r.writeLine("     - 考虑使用 Netty 的 PooledByteBufAllocator 复用缓冲区")
				r.writeLine("     - 检查是否正确调用 Cleaner.clean() 释放底层内存")
				r.writeLine("")
			}
		case AllocSourceUnsafe:
			hasUnsafe = true
			if stat.Percentage > 20 {
				r.writeLine("  2. Unsafe.allocateMemory 泄漏 (%.1f%%):", stat.Percentage)
				r.writeLine("     - 确保 Unsafe.freeMemory() 在 finally 块中调用")
				r.writeLine("     - 考虑使用 ByteBuffer.allocateDirect 替代直接 Unsafe 操作")
				r.writeLine("     - 使用 try-with-resources 或 Cleaner 模式管理生命周期")
				r.writeLine("")
			}
		case AllocSourceJNI:
			hasJNI = true
			if stat.Percentage > 10 {
				r.writeLine("  3. JNI 堆外内存泄漏 (%.1f%%):", stat.Percentage)
				r.writeLine("     - 检查 JNI 代码中的 malloc/free 配对")
				r.writeLine("     - 使用 JNI NewDirectByteBuffer 创建的内存需手动释放")
				r.writeLine("     - 考虑使用 GetPrimitiveArrayCritical 替代 Get<type>ArrayElements")
				r.writeLine("")
			}
		case AllocSourceMappedByteBuffer:
			if stat.Percentage > 10 {
				r.writeLine("  4. MappedByteBuffer 泄漏 (%.1f%%):", stat.Percentage)
				r.writeLine("     - 确保调用了 FileChannel.close() 或 Cleaner.clean()")
				r.writeLine("     - 检查是否重复映射同一文件")
				r.writeLine("")
			}
		}
	}

	if summary.TotalLeakSize > 100*1024*1024 {
		r.writeLine("  5. 堆外内存泄漏总量超过 100MB:")
		r.writeLine("     - 建议立即排查，可能导致 OOM 或触发系统 OOM Killer")
		r.writeLine("     - 使用 jcmd <pid> VM.native_memory summary 查看详细分配")
		r.writeLine("")
	}

	if !hasDirectBuffer && !hasUnsafe && !hasJNI {
		r.writeLine("  当前未检测到严重泄漏，建议:")
		r.writeLine("  - 定期执行堆外内存检测，建立基线")
		r.writeLine("  - 在应用关键路径（如批量数据处理后）检查内存是否释放")
		r.writeLine("")
	}
}

func (r *reportWriter) writeFooter() {
	r.writeLine("=" * 70)
	r.writeLine("报告结束")
	r.writeLine("=" * 70)
}
