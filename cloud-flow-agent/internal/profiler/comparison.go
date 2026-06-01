// Package profiler 提供 ON/OFF-CPU 对比分析功能
// 本文件实现 ON-CPU 和 OFF-CPU 数据的关联分析和对比
package profiler

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"time"
)

// ==================== 对比分析数据结构 ====================

// CPUComparison ON/OFF-CPU 对比分析结果
type CPUComparison struct {
	// 基本信息
	PID         uint32        // 进程 ID
	ProcessName string        // 进程名
	Duration    time.Duration // 分析时长

	// ON-CPU 统计
	OnCPUTotal   int64   // ON-CPU 总时间 (微秒)
	OnCPUPercent float64 // ON-CPU 占比 (%)

	// OFF-CPU 统计
	OffCPUTotal   int64   // OFF-CPU 总时间 (微秒)
	OffCPUPercent float64 // OFF-CPU 占比 (%)

	// 阻塞原因分析
	OffCPUByReason map[OffCPUReason]*ReasonAnalysis // 按原因分类的阻塞分析

	// 热点函数对比
	HotFunctions []FunctionComparison // 函数级对比

	// 时间线分析
	Timeline []TimeSlice // 时间切片分析
}

// ReasonAnalysis 阻塞原因分析
type ReasonAnalysis struct {
	Reason        OffCPUReason // 阻塞原因
	Count         uint64       // 次数
	TotalDuration int64        // 总时长 (微秒)
	AvgDuration   float64      // 平均时长 (微秒)
	Percentage    float64      // 占 OFF-CPU 时间的百分比
	TopStacks     []string     // 最常见的调用栈
}

// FunctionComparison 函数级对比
type FunctionComparison struct {
	FunctionName string  // 函数名
	OnCPUTime    int64   // ON-CPU 时间 (微秒)
	OffCPUTime   int64   // OFF-CPU 时间 (微秒)
	TotalTime    int64   // 总时间
	OnPercent    float64 // ON-CPU 占比
	OffPercent   float64 // OFF-CPU 占比
	Bottleneck   string  // 瓶颈类型: "cpu_bound" | "io_bound" | "lock_bound" | "mixed"
}

// TimeSlice 时间切片
type TimeSlice struct {
	Timestamp   int64   // 时间戳
	OnCPU       bool    // 是否在 CPU 上
	Duration    int64   // 持续时间 (微秒)
	Reason      OffCPUReason // 如果 OFF-CPU，阻塞原因
	Function    string  // 当前函数
}

// ComparisonConfig 对比分析配置
type ComparisonConfig struct {
	// 时间窗口对齐
	AlignTimeWindows bool // 是否对齐 ON/OFF-CPU 的时间窗口

	// 函数匹配
	FunctionMatchThreshold float64 // 函数名匹配阈值 (0-1)

	// 热点分析
	TopNFunctions int // Top N 热点函数
	MinDuration   int64 // 最小持续时间 (微秒)
}

// DefaultComparisonConfig 返回默认配置
func DefaultComparisonConfig() *ComparisonConfig {
	return &ComparisonConfig{
		AlignTimeWindows:       true,
		FunctionMatchThreshold: 0.8,
		TopNFunctions:          20,
		MinDuration:            1000, // 1ms
	}
}

// ==================== 对比分析器 ====================

// CPUComparator ON/OFF-CPU 对比分析器
type CPUComparator struct {
	config *ComparisonConfig
}

// NewCPUComparator 创建对比分析器
func NewCPUComparator(cfg *ComparisonConfig) *CPUComparator {
	if cfg == nil {
		cfg = DefaultComparisonConfig()
	}
	return &CPUComparator{config: cfg}
}

// Compare 执行 ON/OFF-CPU 对比分析
// 参数:
//   - onCPUStacks: ON-CPU 采样栈计数 (stack_key -> count)
//   - offCPUEvents: OFF-CPU 事件列表
//   - duration: 分析时长
func (c *CPUComparator) Compare(
	onCPUStacks map[string]uint64,
	offCPUEvents []*OffCPUEvent,
	duration time.Duration,
) (*CPUComparison, error) {

	if len(onCPUStacks) == 0 && len(offCPUEvents) == 0 {
		return nil, fmt.Errorf("没有 ON-CPU 或 OFF-CPU 数据")
	}

	comparison := &CPUComparison{
		Duration:       duration,
		OffCPUByReason: make(map[OffCPUReason]*ReasonAnalysis),
	}

	// 1. 计算 ON-CPU 统计
	onCPUTotal := c.calculateOnCPUTotal(onCPUStacks)
	comparison.OnCPUTotal = onCPUTotal

	// 2. 计算 OFF-CPU 统计
	offCPUTotal := c.calculateOffCPUTotal(offCPUEvents)
	comparison.OffCPUTotal = offCPUTotal

	// 3. 计算百分比	total := onCPUTotal + offCPUTotal
	if total > 0 {
		comparison.OnCPUPercent = float64(onCPUTotal) * 100.0 / float64(total)
		comparison.OffCPUPercent = float64(offCPUTotal) * 100.0 / float64(total)
	}

	// 4. 分析阻塞原因
	c.analyzeOffCPUByReason(comparison, offCPUEvents)

	// 5. 函数级对比
	c.compareFunctions(comparison, onCPUStacks, offCPUEvents)

	// 6. 生成时间线
	c.generateTimeline(comparison, onCPUStacks, offCPUEvents)

	return comparison, nil
}

// calculateOnCPUTotal 计算 ON-CPU 总时间
func (c *CPUComparator) calculateOnCPUTotal(stacks map[string]uint64) int64 {
	var total uint64
	for _, count := range stacks {
		total += count
	}
	// 假设每个采样间隔为 10ms (100Hz)
	return int64(total) * 10000 // 转换为微秒
}

// calculateOffCPUTotal 计算 OFF-CPU 总时间
func (c *CPUComparator) calculateOffCPUTotal(events []*OffCPUEvent) int64 {
	var total int64
	for _, event := range events {
		total += event.Duration
	}
	return total
}

// analyzeOffCPUByReason 按原因分析 OFF-CPU
func (c *CPUComparator) analyzeOffCPUByReason(comparison *CPUComparison, events []*OffCPUEvent) {
	reasonStats := make(map[OffCPUReason]*reasonStat)
	reasonStacks := make(map[OffCPUReason]map[string]uint64)

	for _, event := range events {
		if _, ok := reasonStats[event.Reason]; !ok {
			reasonStats[event.Reason] = &reasonStat{}
			reasonStacks[event.Reason] = make(map[string]uint64)
		}

		stat := reasonStats[event.Reason]
		stat.count++
		stat.totalDuration += event.Duration

		// 记录调用栈
		stackKey := c.stackToKey(event.StackTrace)
		reasonStacks[event.Reason][stackKey]++
	}

	// 转换为分析结果
	for reason, stat := range reasonStats {
		analysis := &ReasonAnalysis{
			Reason:        reason,
			Count:         stat.count,
			TotalDuration: stat.totalDuration,
			AvgDuration:   float64(stat.totalDuration) / float64(stat.count),
		}

		if comparison.OffCPUTotal > 0 {
			analysis.Percentage = float64(stat.totalDuration) * 100.0 / float64(comparison.OffCPUTotal)
		}

		// 获取最常见的调用栈
		analysis.TopStacks = c.getTopStacks(reasonStacks[reason], 3)

		comparison.OffCPUByReason[reason] = analysis
	}
}

// reasonStat 原因统计
type reasonStat struct {
	count         uint64
	totalDuration int64
}

// stackToKey 将调用栈转换为 key
func (c *CPUComparator) stackToKey(stack []string) string {
	if len(stack) == 0 {
		return "unknown"
	}
	// 取前 3 个栈帧作为 key
	var parts []string
	for i, frame := range stack {
		if i >= 3 {
			break
		}
		parts = append(parts, frame)
	}
	return fmt.Sprintf("%v", parts)
}

// getTopStacks 获取最常见的调用栈
func (c *CPUComparator) getTopStacks(stacks map[string]uint64, n int) []string {
	type stackCount struct {
		stack string
		count uint64
	}

	var list []stackCount
	for stack, count := range stacks {
		list = append(list, stackCount{stack, count})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].count > list[j].count
	})

	var result []string
	for i, sc := range list {
		if i >= n {
			break
		}
		result = append(result, sc.stack)
	}
	return result
}

// compareFunctions 函数级对比
func (c *CPUComparator) compareFunctions(
	comparison *CPUComparison,
	onCPUStacks map[string]uint64,
	offCPUEvents []*OffCPUEvent,
) {
	functionMap := make(map[string]*FunctionComparison)

	// 统计 ON-CPU 时间
	for stackKey, count := range onCPUStacks {
		functions := c.parseStack(stackKey)
		if len(functions) == 0 {
			continue
		}

		// 将时间分配给栈中的每个函数
		onCPUTime := int64(count) * 10000 // 转换为微秒
		perFuncTime := onCPUTime / int64(len(functions))

		for _, fn := range functions {
			if _, ok := functionMap[fn]; !ok {
				functionMap[fn] = &FunctionComparison{FunctionName: fn}
			}
			functionMap[fn].OnCPUTime += perFuncTime
		}
	}

	// 统计 OFF-CPU 时间
	for _, event := range offCPUEvents {
		functions := event.StackTrace
		if len(functions) == 0 {
			continue
		}

		// 将阻塞时间分配给栈中的每个函数
		offCPUTime := event.Duration
		perFuncTime := offCPUTime / int64(len(functions))

		for _, fn := range functions {
			if _, ok := functionMap[fn]; !ok {
				functionMap[fn] = &FunctionComparison{FunctionName: fn}
			}
			functionMap[fn].OffCPUTime += perFuncTime
			functionMap[fn].OffPercent = float64(functionMap[fn].OffCPUTime) * 100.0 / float64(comparison.OffCPUTotal)
		}
	}

	// 计算总时间和百分比
	totalTime := comparison.OnCPUTotal + comparison.OffCPUTotal
	for _, fn := range functionMap {
		fn.TotalTime = fn.OnCPUTime + fn.OffCPUTime
		if totalTime > 0 {
			fn.OnPercent = float64(fn.OnCPUTime) * 100.0 / float64(totalTime)
			fn.OffPercent = float64(fn.OffCPUTime) * 100.0 / float64(totalTime)
		}
		fn.Bottleneck = c.classifyBottleneck(fn)
	}

	// 转换为列表并排序
	var list []FunctionComparison
	for _, fn := range functionMap {
		if fn.TotalTime >= c.config.MinDuration {
			list = append(list, *fn)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].TotalTime > list[j].TotalTime
	})

	// 取 Top N
	if c.config.TopNFunctions > 0 && len(list) > c.config.TopNFunctions {
		list = list[:c.config.TopNFunctions]
	}

	comparison.HotFunctions = list
}

// parseStack 解析栈 key
func (c *CPUComparator) parseStack(stackKey string) []string {
	// 简化实现：假设 stackKey 是用 ";" 分隔的函数名
	// 实际实现需要根据实际格式解析
	return nil
}

// classifyBottleneck 分类瓶颈类型
func (c *CPUComparator) classifyBottleneck(fn *FunctionComparison) string {
	onRatio := float64(fn.OnCPUTime) / float64(fn.TotalTime)
	offRatio := float64(fn.OffCPUTime) / float64(fn.TotalTime)

	if onRatio > 0.7 {
		return "cpu_bound" // CPU 密集型
	}
	if offRatio > 0.7 {
		return "io_bound" // IO 密集型
	}
	if fn.OffCPUTime > 0 && fn.OnCPUTime > 0 {
		return "mixed" // 混合型
	}
	return "unknown"
}

// generateTimeline 生成时间线
func (c *CPUComparator) generateTimeline(
	comparison *CPUComparison,
	onCPUStacks map[string]uint64,
	offCPUEvents []*OffCPUEvent,
) {
	// 简化实现：生成时间切片
	// 实际实现需要根据时间戳生成详细的时间线

	var timeline []TimeSlice

	// 添加 OFF-CPU 事件
	for _, event := range offCPUEvents {
		timeline = append(timeline, TimeSlice{
			Timestamp: event.OffCPUTime,
			OnCPU:     false,
			Duration:  event.Duration,
			Reason:    event.Reason,
			Function:  c.getTopFunction(event.StackTrace),
		})
	}

	// 按时间排序
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp < timeline[j].Timestamp
	})

	comparison.Timeline = timeline
}

// getTopFunction 获取栈顶函数
func (c *CPUComparator) getTopFunction(stack []string) string {
	if len(stack) > 0 {
		return stack[0]
	}
	return "unknown"
}

// ==================== 报告生成 ====================

// GenerateReport 生成对比分析报告
func (c *CPUComparator) GenerateReport(comparison *CPUComparison, output io.Writer) error {
	fmt.Fprintln(output, "="*60)
	fmt.Fprintln(output, "ON/OFF-CPU 对比分析报告")
	fmt.Fprintln(output, "="*60)
	fmt.Fprintln(output)

	// 基本信息
	fmt.Fprintf(output, "进程: %s (PID: %d)\n", comparison.ProcessName, comparison.PID)
	fmt.Fprintf(output, "分析时长: %v\n", comparison.Duration)
	fmt.Fprintln(output)

	// CPU 时间分布
	fmt.Fprintln(output, "【CPU 时间分布】")
	fmt.Fprintf(output, "  ON-CPU:  %10.2f ms (%.1f%%)\n",
		float64(comparison.OnCPUTotal)/1000.0, comparison.OnCPUPercent)
	fmt.Fprintf(output, "  OFF-CPU: %10.2f ms (%.1f%%)\n",
		float64(comparison.OffCPUTotal)/1000.0, comparison.OffCPUPercent)
	fmt.Fprintln(output)

	// 阻塞原因分析
	fmt.Fprintln(output, "【阻塞原因分析】")
	for reason, analysis := range comparison.OffCPUByReason {
		fmt.Fprintf(output, "  %s:\n", reason)
		fmt.Fprintf(output, "    次数: %d, 总时长: %.2f ms, 平均: %.2f ms, 占比: %.1f%%\n",
			analysis.Count,
			float64(analysis.TotalDuration)/1000.0,
			analysis.AvgDuration/1000.0,
			analysis.Percentage)
		if len(analysis.TopStacks) > 0 {
			fmt.Fprintln(output, "    常见调用栈:")
			for _, stack := range analysis.TopStacks {
				fmt.Fprintf(output, "      - %s\n", stack)
			}
		}
	}
	fmt.Fprintln(output)

	// 热点函数对比
	fmt.Fprintln(output, "【热点函数对比 (Top 20)】")
	fmt.Fprintf(output, "%-40s %10s %10s %10s %12s\n",
		"函数名", "ON-CPU(ms)", "OFF-CPU(ms)", "瓶颈类型", "占比")
	fmt.Fprintln(output, "-"*90)
	for i, fn := range comparison.HotFunctions {
		if i >= 20 {
			break
		}
		fmt.Fprintf(output, "%-40s %10.2f %10.2f %10s %11.1f%%\n",
			c.truncateString(fn.FunctionName, 40),
			float64(fn.OnCPUTime)/1000.0,
			float64(fn.OffCPUTime)/1000.0,
			fn.Bottleneck,
			fn.OnPercent+fn.OffPercent)
	}
	fmt.Fprintln(output)

	// 优化建议
	fmt.Fprintln(output, "【优化建议】")
	c.generateRecommendations(comparison, output)

	return nil
}

// generateRecommendations 生成优化建议
func (c *CPUComparator) generateRecommendations(comparison *CPUComparison, output io.Writer) {
	// 根据分析结果生成优化建议

	// 1. IO 等待优化
	if ioAnalysis, ok := comparison.OffCPUByReason[OffCPUReasonIOWait]; ok {
		if ioAnalysis.Percentage > 30 {
			fmt.Fprintln(output, "  1. IO 等待占比较高 (" + fmt.Sprintf("%.1f%%", ioAnalysis.Percentage) + "):")
			fmt.Fprintln(output, "     - 考虑使用异步 IO 或 IO 多路复用")
			fmt.Fprintln(output, "     - 检查磁盘性能，考虑使用 SSD 或优化文件系统")
			fmt.Fprintln(output, "     - 增加缓存，减少磁盘访问")
			fmt.Fprintln(output)
		}
	}

	// 2. 锁竞争优化
	if lockAnalysis, ok := comparison.OffCPUByReason[OffCPUReasonLockContention]; ok {
		if lockAnalysis.Percentage > 20 {
			fmt.Fprintln(output, "  2. 锁竞争较严重 (" + fmt.Sprintf("%.1f%%", lockAnalysis.Percentage) + "):")
			fmt.Fprintln(output, "     - 检查热点锁，考虑使用读写锁或无锁数据结构")
			fmt.Fprintln(output, "     - 减小锁粒度，缩短临界区")
			fmt.Fprintln(output, "     - 考虑使用 Lock-Free 算法")
			fmt.Fprintln(output)
		}
	}

	// 3. 调度延迟优化
	if schedAnalysis, ok := comparison.OffCPUByReason[OffCPUReasonScheduler]; ok {
		if schedAnalysis.Percentage > 15 {
			fmt.Fprintln(output, "  3. 调度延迟较高 (" + fmt.Sprintf("%.1f%%", schedAnalysis.Percentage) + "):")
			fmt.Fprintln(output, "     - 检查系统负载，可能需要增加 CPU 核心")
			fmt.Fprintln(output, "     - 调整进程优先级或使用实时调度策略")
			fmt.Fprintln(output, "     - 检查是否有 CPU 密集型任务影响调度")
			fmt.Fprintln(output)
		}
	}

	// 4. CPU 密集型优化
	if comparison.OnCPUPercent > 80 {
		fmt.Fprintln(output, "  4. CPU 使用率较高 (" + fmt.Sprintf("%.1f%%", comparison.OnCPUPercent) + "):")
		fmt.Fprintln(output, "     - 考虑算法优化，降低时间复杂度")
		fmt.Fprintln(output, "     - 使用性能剖析工具定位热点函数")
		fmt.Fprintln(output, "     - 考虑并行化处理")
		fmt.Fprintln(output)
	}
}

// truncateString 截断字符串
func (c *CPUComparator) truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen > 3 {
		return s[:maxLen-3] + "..."
	}
	return s[:maxLen]
}

// ToJSON 导出为 JSON 格式
func (c *CPUComparator) ToJSON(comparison *CPUComparison) ([]byte, error) {
	// 简化实现，实际应该使用 json.Marshal
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{\n")
	fmt.Fprintf(&buf, "  \"pid\": %d,\n", comparison.PID)
	fmt.Fprintf(&buf, "  \"process_name\": \"%s\",\n", comparison.ProcessName)
	fmt.Fprintf(&buf, "  \"duration\": \"%s\",\n", comparison.Duration)
	fmt.Fprintf(&buf, "  \"on_cpu_percent\": %.2f,\n", comparison.OnCPUPercent)
	fmt.Fprintf(&buf, "  \"off_cpu_percent\": %.2f,\n", comparison.OffCPUPercent)
	fmt.Fprintf(&buf, "  \"hot_functions_count\": %d\n", len(comparison.HotFunctions))
	fmt.Fprintf(&buf, "}\n")
	return buf.Bytes(), nil
}
