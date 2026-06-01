// Package profiler 提供 OFF-CPU 性能剖析功能
// 本文件实现 OFF-CPU 火焰图生成器，展示阻塞原因的分布
package profiler

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ==================== OFF-CPU 火焰图生成器 ====================

// OffCPUFlameGraph OFF-CPU 火焰图生成器
// 与 ON-CPU 火焰图不同，OFF-CPU 火焰图展示的是阻塞原因的分布
type OffCPUFlameGraph struct {
	// 配置选项
	Width        int     // SVG 画布宽度
	Height       int     // SVG 画布高度，0 表示自动计算
	MinWidth     float64 // 最小栈帧宽度
	Title        string  // 标题
	Subtitle     string  // 副标题（显示统计信息）
	FontFamily   string  // 字体族
	FontSize     int     // 字体大小
	FontColor    string  // 字体颜色

	// 颜色配置（按阻塞原因使用不同颜色）
	ColorScheme map[OffCPUReason]string // 阻塞原因 -> 颜色映射
}

// OffCPUFlameNode OFF-CPU 火焰图节点
type offCPUFlameNode struct {
	name     string           // 函数名/阻塞原因
	reason   OffCPUReason     // 阻塞原因
	value    uint64           // 自身采样数
	total    uint64           // 总采样数
	children []*offCPUFlameNode // 子节点
}

// NewOffCPUFlameGraph 创建 OFF-CPU 火焰图生成器
func NewOffCPUFlameGraph() *OffCPUFlameGraph {
	return &OffCPUFlameGraph{
		Width:      1200,
		MinWidth:   0.1,
		Title:      "OFF-CPU 火焰图",
		FontFamily: "Verdana, sans-serif",
		FontSize:   11,
		FontColor:  "rgb(0,0,0)",
		ColorScheme: map[OffCPUReason]string{
			OffCPUReasonIOWait:         "hsl(200, 70%, 60%)",   // 蓝色 - IO等待
			OffCPUReasonLockContention: "hsl(0, 70%, 60%)",     // 红色 - 锁竞争
			OffCPUReasonScheduler:      "hsl(120, 70%, 60%)",   // 绿色 - 调度延迟
			OffCPUReasonNetwork:        "hsl(280, 70%, 60%)",   // 紫色 - 网络等待
			OffCPUReasonDisk:           "hsl(30, 70%, 60%)",    // 橙色 - 磁盘等待
			OffCPUReasonFutex:          "hsl(60, 70%, 60%)",    // 黄色 - futex
			OffCPUReasonPipe:           "hsl(180, 70%, 60%)",   // 青色 - 管道
			OffCPUReasonPoll:           "hsl(240, 70%, 60%)",   // 蓝色 - poll
			OffCPUReasonSleep:          "hsl(300, 70%, 60%)",   // 品红 - 睡眠
			OffCPUReasonUnknown:        "hsl(0, 0%, 60%)",      // 灰色 - 未知
		},
	}
}

// Generate 生成 OFF-CPU 火焰图
// 参数:
//   - events: OFF-CPU 事件列表
//   - output: SVG 输出流
func (fg *OffCPUFlameGraph) Generate(events []*OffCPUEvent, output io.Writer) error {
	if len(events) == 0 {
		return fmt.Errorf("没有 OFF-CPU 事件数据")
	}

	// 1. 构建火焰图树
	root := fg.buildTree(events)

	// 2. 计算总阻塞时长
	totalDuration := root.total
	if totalDuration == 0 {
		return fmt.Errorf("总阻塞时长为 0")
	}

	// 3. 计算画布高度
	maxDepth := fg.calculateMaxDepth(root)
	frameHeight := 16
	padTop := 20
	padBottom := 60 // 更大的底部空间显示统计信息
	titleHeight := 30

	canvasHeight := fg.Height
	if canvasHeight == 0 {
		canvasHeight = titleHeight + padTop + (maxDepth+1)*frameHeight + padBottom
	}

	// 4. 生成统计信息
	stats := fg.generateStats(events)

	// 5. 生成 SVG
	fg.writeSVG(output, root, totalDuration, maxDepth, frameHeight, padTop, padBottom, titleHeight, canvasHeight, stats)

	return nil
}

// buildTree 从事件列表构建火焰图树
// 栈结构: 根 -> 阻塞原因 -> 调用栈
func (fg *OffCPUFlameGraph) buildTree(events []*OffCPUEvent) *offCPUFlameNode {
	root := &offCPUFlameNode{
		name:     "all",
		reason:   OffCPUReasonUnknown,
		value:    0,
		total:    0,
		children: make([]*offCPUFlameNode, 0),
	}

	for _, event := range events {
		duration := uint64(event.Duration)
		if duration == 0 {
			duration = 1
		}

		// 构建栈帧链: 阻塞原因 + 调用栈
		var frames []string
		frames = append(frames, string(event.Reason))
		frames = append(frames, event.StackTrace...)

		// 从根节点开始构建树
		current := root
		for i, frame := range frames {
			if frame == "" {
				continue
			}

			// 查找是否已存在该子节点
			found := false
			for _, child := range current.children {
				if child.name == frame {
					current = child
					found = true
					break
				}
			}

			// 如果不存在，创建新节点
			if !found {
				newNode := &offCPUFlameNode{
					name:     frame,
					reason:   event.Reason,
					value:    0,
					total:    0,
					children: make([]*offCPUFlameNode, 0),
				}
				// 第一层节点设置阻塞原因
				if i == 0 {
					newNode.reason = event.Reason
				}
				current.children = append(current.children, newNode)
				current = newNode
			}
		}

		// 叶节点增加阻塞时长
		current.value += duration
	}

	// 自底向上计算每个节点的总时长
	fg.calculateTotals(root)

	return root
}

// calculateTotals 递归计算每个节点的总时长
func (fg *OffCPUFlameGraph) calculateTotals(node *offCPUFlameNode) uint64 {
	if len(node.children) == 0 {
		node.total = node.value
		return node.total
	}

	var childTotal uint64
	for _, child := range node.children {
		childTotal += fg.calculateTotals(child)
	}

	node.total = node.value + childTotal
	return node.total
}

// calculateMaxDepth 计算树的最大深度
func (fg *OffCPUFlameGraph) calculateMaxDepth(node *offCPUFlameNode) int {
	if len(node.children) == 0 {
		return 0
	}

	maxChildDepth := 0
	for _, child := range node.children {
		depth := fg.calculateMaxDepth(child)
		if depth > maxChildDepth {
			maxChildDepth = depth
		}
	}

	return maxChildDepth + 1
}

// generateStats 生成统计信息
func (fg *OffCPUFlameGraph) generateStats(events []*OffCPUEvent) map[string]interface{} {
	stats := make(map[string]interface{})

	var totalDuration int64
	reasonCounts := make(map[OffCPUReason]uint64)
	reasonDurations := make(map[OffCPUReason]int64)

	for _, event := range events {
		totalDuration += event.Duration
		reasonCounts[event.Reason]++
		reasonDurations[event.Reason] += event.Duration
	}

	stats["total_events"] = len(events)
	stats["total_duration_ms"] = totalDuration / 1000
	stats["avg_duration_ms"] = totalDuration / int64(len(events)) / 1000
	stats["reason_counts"] = reasonCounts
	stats["reason_durations_ms"] = reasonDurations

	return stats
}

// writeSVG 生成 SVG 火焰图
func (fg *OffCPUFlameGraph) writeSVG(output io.Writer, root *offCPUFlameNode, totalDuration uint64,
	maxDepth int, frameHeight, padTop, padBottom, titleHeight, canvasHeight int,
	stats map[string]interface{}) {

	drawWidth := fg.Width - 20

	// 写入 SVG 头部
	fmt.Fprintf(output, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, fg.Width, canvasHeight, fg.Width, canvasHeight)
	fmt.Fprintln(output)

	// 写入 CSS 样式
	fg.writeStyles(output)

	// 写入 JavaScript
	fg.writeJavaScript(output, totalDuration)

	// 写入标题
	fmt.Fprintf(output, `<text x="%d" y="20" text-anchor="middle" font-family="%s" font-size="16" fill="rgb(0,0,0)">%s</text>`,
		fg.Width/2, fg.FontFamily, fg.Title)
	fmt.Fprintln(output)

	// 写入副标题（统计信息）
	totalEvents := stats["total_events"].(int)
	totalDurationMs := stats["total_duration_ms"].(int64)
	fmt.Fprintf(output, `<text x="%d" y="35" text-anchor="middle" font-family="%s" font-size="11" fill="rgb(100,100,100)">事件数: %d | 总阻塞: %d ms</text>`,
		fg.Width/2, fg.FontFamily, totalEvents, totalDurationMs)
	fmt.Fprintln(output)

	// 写入图例
	fg.writeLegend(output, titleHeight+5)

	// 写入火焰图主体
	fmt.Fprintf(output, `<g class="flamegraph">`)
	fmt.Fprintln(output)
	fg.writeFlameRects(output, root, 10, titleHeight+padTop+30, drawWidth, totalDuration, frameHeight, 0)
	fmt.Fprintf(output, `</g>`)
	fmt.Fprintln(output)

	// 写入底部统计信息
	fg.writeStats(output, stats, canvasHeight-padBottom+20)

	// 写入 SVG 结束标签
	fmt.Fprintf(output, `</svg>`)
}

// writeStyles 写入 CSS 样式
func (fg *OffCPUFlameGraph) writeStyles(output io.Writer) {
	fmt.Fprintln(output, `<style>`)
	fmt.Fprintln(output, `  .flamegraph rect { cursor: pointer; stroke: rgb(0,0,0); stroke-width: 0.5; }`)
	fmt.Fprintln(output, `  .flamegraph rect:hover { stroke: rgb(0,0,255); stroke-width: 1.5; }`)
	fmt.Fprintln(output, `  .flamegraph rect.dimmed { opacity: 0.3; }`)
	fmt.Fprintln(output, `  .flamegraph rect.highlighted { stroke: rgb(0,0,255); stroke-width: 2; }`)
	fmt.Fprintln(output, `  .flamegraph text { pointer-events: none; }`)
	fmt.Fprintln(output, `  .legend rect { stroke: rgb(0,0,0); stroke-width: 0.5; }`)
	fmt.Fprintln(output, `  .legend text { font-family: Verdana, sans-serif; font-size: 10px; }`)
	fmt.Fprintln(output, `</style>`)
}

// writeJavaScript 写入 JavaScript
func (fg *OffCPUFlameGraph) writeJavaScript(output io.Writer, totalDuration uint64) {
	fmt.Fprintln(output, `<script type="text/ecmascript"><![CDATA[`)
	fmt.Fprintln(output, `  function searchFlamegraph(query) {`)
	fmt.Fprintln(output, `    var rects = document.querySelectorAll('.flamegraph rect');`)
	fmt.Fprintln(output, `    if (query === '') {`)
	fmt.Fprintln(output, `      for (var i = 0; i < rects.length; i++) {`)
	fmt.Fprintln(output, `        rects[i].classList.remove('dimmed', 'highlighted');`)
	fmt.Fprintln(output, `      }`)
	fmt.Fprintln(output, `      return;`)
	fmt.Fprintln(output, `    }`)
	fmt.Fprintln(output, `    query = query.toLowerCase();`)
	fmt.Fprintln(output, `    for (var i = 0; i < rects.length; i++) {`)
	fmt.Fprintln(output, `      var title = rects[i].getAttribute('title') || '';`)
	fmt.Fprintln(output, `      if (title.toLowerCase().indexOf(query) >= 0) {`)
	fmt.Fprintln(output, `        rects[i].classList.add('highlighted');`)
	fmt.Fprintln(output, `        rects[i].classList.remove('dimmed');`)
	fmt.Fprintln(output, `      } else {`)
	fmt.Fprintln(output, `        rects[i].classList.add('dimmed');`)
	fmt.Fprintln(output, `        rects[i].classList.remove('highlighted');`)
	fmt.Fprintln(output, `      }`)
	fmt.Fprintln(output, `    }`)
	fmt.Fprintln(output, `  }`)
	fmt.Fprintln(output, `]]></script>`)
}

// writeLegend 写入图例
func (fg *OffCPUFlameGraph) writeLegend(output io.Writer, y int) {
	fmt.Fprintf(output, `<g class="legend">`)
	x := 10
	for reason, color := range fg.ColorScheme {
		// 只显示有定义的颜色
		if x > fg.Width-100 {
			break
		}
		fmt.Fprintf(output, `<rect x="%d" y="%d" width="12" height="12" fill="%s"/>`, x, y, color)
		fmt.Fprintf(output, `<text x="%d" y="%d">%s</text>`, x+15, y+10, reason)
		x += 80
	}
	fmt.Fprintf(output, `</g>`)
	fmt.Fprintln(output)
}

// writeFlameRects 递归绘制火焰图矩形
func (fg *OffCPUFlameGraph) writeFlameRects(output io.Writer, node *offCPUFlameNode, x, y, width int,
	totalDuration uint64, frameHeight, depth int) {

	if float64(width) < fg.MinWidth {
		return
	}

	percentage := float64(node.total) * 100.0 / float64(totalDuration)
	pctStr := fmt.Sprintf("%.2f%%", percentage)

	// 获取颜色（第一层节点根据阻塞原因着色，后续节点继承）
	color := fg.getColor(node, depth)

	// 生成悬停提示
	durationMs := float64(node.total) / 1000.0
	title := fmt.Sprintf("%s (%.2f ms, %s)", node.name, durationMs, pctStr)

	// 写入矩形
	fmt.Fprintf(output, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" title="%s"/>`,
		x, y, width, frameHeight, color, title)
	fmt.Fprintln(output)

	// 写入文字
	if width > 30 {
		displayName := fg.truncateText(node.name, width-6)
		fmt.Fprintf(output, `<text x="%d" y="%d" font-family="%s" font-size="%d" fill="%s">%s</text>`,
			x+3, y+frameHeight-3, fg.FontFamily, fg.FontSize, fg.FontColor, displayName)
		fmt.Fprintln(output)
	}

	// 递归绘制子节点
	childX := x
	for _, child := range node.children {
		childWidth := float64(child.total) * float64(width) / float64(node.total)
		childWidthInt := int(childWidth)

		if childWidthInt > 0 {
			fg.writeFlameRects(output, child, childX, y+frameHeight, childWidthInt,
				totalDuration, frameHeight, depth+1)
			childX += childWidthInt
		}
	}
}

// getColor 获取节点颜色
func (fg *OffCPUFlameGraph) getColor(node *offCPUFlameNode, depth int) string {
	// 第一层节点根据阻塞原因着色
	if depth == 1 {
		if color, ok := fg.ColorScheme[node.reason]; ok {
			return color
		}
	}

	// 后续节点继承父节点颜色但调整亮度
	if depth > 1 && node.reason != OffCPUReasonUnknown {
		if color, ok := fg.ColorScheme[node.reason]; ok {
			// 调整亮度（深度越深，颜色越浅）
			return fg.adjustBrightness(color, depth)
		}
	}

	// 默认颜色
	return fg.ColorScheme[OffCPUReasonUnknown]
}

// adjustBrightness 调整颜色亮度
func (fg *OffCPUFlameGraph) adjustBrightness(color string, depth int) string {
	// 简化实现：直接返回原颜色
	// 实际实现应该解析 HSL 并调整 L 值
	return color
}

// truncateText 截断文本
func (fg *OffCPUFlameGraph) truncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	charWidth := float64(fg.FontSize) * 0.6
	maxChars := int(float64(maxWidth) / charWidth)

	if len(text) <= maxChars {
		return text
	}

	if maxChars > 3 {
		return text[:maxChars-3] + "..."
	}
	return text[:maxChars]
}

// writeStats 写入统计信息
func (fg *OffCPUFlameGraph) writeStats(output io.Writer, stats map[string]interface{}, y int) {
	reasonDurations := stats["reason_durations_ms"].(map[OffCPUReason]int64)

	// 按阻塞时长排序
	type reasonStat struct {
		reason    OffCPUReason
		duration  int64
	}
	var sortedStats []reasonStat
	for reason, duration := range reasonDurations {
		sortedStats = append(sortedStats, reasonStat{reason, duration})
	}
	sort.Slice(sortedStats, func(i, j int) bool {
		return sortedStats[i].duration > sortedStats[j].duration
	})

	// 显示 Top 5
	fmt.Fprintf(output, `<text x="10" y="%d" font-family="%s" font-size="10" fill="rgb(100,100,100)">Top 阻塞原因:</text>`, y, fg.FontFamily)
	x := 100
	for i, stat := range sortedStats {
		if i >= 5 || x > fg.Width-100 {
			break
		}
		fmt.Fprintf(output, `<text x="%d" y="%d" font-family="%s" font-size="10" fill="%s">%s: %d ms</text>`,
			x, y, fg.FontFamily, fg.ColorScheme[stat.reason], stat.reason, stat.duration)
		x += 120
	}
}

// ==================== 热点阻塞分析 ====================

// OffCPUHotSpot OFF-CPU 热点信息
type OffCPUHotSpot struct {
	Name       string       // 函数名/阻塞点
	Reason     OffCPUReason // 阻塞原因
	Count      uint64       // 阻塞次数
	Duration   int64        // 总阻塞时长 (微秒)
	AvgDuration float64     // 平均阻塞时长
	Percentage float64      // 占比
}

// GenerateHotSpots 生成热点阻塞列表
func (fg *OffCPUFlameGraph) GenerateHotSpots(events []*OffCPUEvent, topN int) []OffCPUHotSpot {
	if len(events) == 0 {
		return nil
	}

	// 统计每个阻塞点的总时长
	hotspotMap := make(map[string]*OffCPUHotSpot)
	var totalDuration int64

	for _, event := range events {
		totalDuration += event.Duration

		// 使用栈顶函数作为阻塞点标识
		key := string(event.Reason)
		if len(event.StackTrace) > 0 {
			key = event.StackTrace[0] + " (" + string(event.Reason) + ")"
		}

		if _, ok := hotspotMap[key]; !ok {
			hotspotMap[key] = &OffCPUHotSpot{
				Name:   key,
				Reason: event.Reason,
			}
		}

		spot := hotspotMap[key]
		spot.Count++
		spot.Duration += event.Duration
	}

	// 转换为列表并计算百分比
	var hotspots []OffCPUHotSpot
	for _, spot := range hotspotMap {
		spot.AvgDuration = float64(spot.Duration) / float64(spot.Count)
		spot.Percentage = float64(spot.Duration) * 100.0 / float64(totalDuration)
		hotspots = append(hotspots, *spot)
	}

	// 按总时长排序
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].Duration > hotspots[j].Duration
	})

	// 返回 Top N
	if topN > 0 && len(hotspots) > topN {
		return hotspots[:topN]
	}
	return hotspots
}

// GenerateByReason 按阻塞原因生成火焰图
func (fg *OffCPUFlameGraph) GenerateByReason(events []*OffCPUEvent, reason OffCPUReason, output io.Writer) error {
	// 过滤指定原因的事件
	var filtered []*OffCPUEvent
	for _, event := range events {
		if event.Reason == reason {
			filtered = append(filtered, event)
		}
	}

	if len(filtered) == 0 {
		return fmt.Errorf("没有 %s 类型的阻塞事件", reason)
	}

	// 临时修改标题
	originalTitle := fg.Title
	fg.Title = fmt.Sprintf("OFF-CPU 火焰图 - %s", reason)
	defer func() { fg.Title = originalTitle }()

	return fg.Generate(filtered, output)
}
