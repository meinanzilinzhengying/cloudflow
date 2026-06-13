// Package profiler 提供 ON-CPU 性能剖析功能
// 本文件实现火焰图生成器，将栈采样数据转换为交互式 SVG 火焰图
package profiler

import (
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strings"
)

// ==================== 火焰图相关结构体 ====================

// HotFunction 表示一个热点函数
// 包含函数名、源文件位置、采样数和占比信息
type HotFunction struct {
	Name       string  // 函数名
	File       string  // 源代码文件路径
	Line       int     // 源代码行号
	Samples    uint64  // 采样次数
	Percentage float64 // 采样百分比 (0-100)
}

// FlameGraph 火焰图生成器
// 将栈采样数据 (栈帧链 + 采样计数) 转换为 SVG 格式的火焰图
// 火焰图是一种可视化 CPU 性能剖析数据的常用方法
type FlameGraph struct {
	// 火焰图配置选项
	Width       int    // SVG 画布宽度 (像素)
	Height      int    // SVG 画布高度 (像素)，0 表示自动计算
	MinWidth    float64 // 最小栈帧宽度 (像素)，低于此值不显示
	Title       string // 火焰图标题
	FontFamily  string // 字体族
	FontSize    int    // 字体大小
	FontColor   string // 字体颜色
}

// flameNode 表示火焰图中的一个节点 (栈帧)
// 用于构建火焰图的树形结构
type flameNode struct {
	name     string   // 函数名
	value    uint64   // 自身采样数 (不含子节点)
	total    uint64   // 总采样数 (含子节点)
	children []*flameNode // 子节点列表
}

// ==================== 构造函数 ====================

// NewFlameGraph 创建一个新的火焰图生成器
// 使用默认配置参数
func NewFlameGraph() *FlameGraph {
	return &FlameGraph{
		Width:       1200,             // 默认宽度 1200 像素
		MinWidth:    0.1,              // 最小栈帧宽度 0.1 像素
		Title:       "CPU 火焰图",      // 默认标题
		FontFamily:  "Verdana, sans-serif", // 默认字体
		FontSize:    11,               // 默认字体大小
		FontColor:   "rgb(0,0,0)",     // 默认字体颜色 (黑色)
	}
}

// ==================== 核心生成函数 ====================

// Generate 生成火焰图并写入输出流
// 参数:
//   - stackCounts: 栈合并计数 map
//     key = 栈帧链，格式为 "main;foo;bar" (分号分隔，从根到叶)
//     value = 采样次数
//   - output: SVG 输出流
//
// 火焰图特性:
//   - 每个栈帧是一个矩形，宽度按采样比例缩放
//   - 颜色使用暖色调 (红->橙->黄)，基于函数名 hash 确定具体颜色
//   - 鼠标悬停显示: 函数名、采样数、百分比
//   - 支持搜索高亮功能
func (fg *FlameGraph) Generate(stackCounts map[string]uint64, output io.Writer) error {
	if len(stackCounts) == 0 {
		return fmt.Errorf("栈采样数据为空，无法生成火焰图")
	}

	// 1. 构建火焰图树
	root := fg.buildTree(stackCounts)

	// 2. 计算总采样数
	totalSamples := root.total
	if totalSamples == 0 {
		return fmt.Errorf("总采样数为 0，无法生成火焰图")
	}

	// 3. 计算画布高度
	// 高度 = 标题区域 + 火焰图区域 + 底部信息区域
	maxDepth := fg.calculateMaxDepth(root)
	frameHeight := 16 // 每个栈帧的高度 (像素)
	padTop := 20      // 顶部内边距
	padBottom := 40   // 底部内边距 (显示搜索框)
	titleHeight := 30 // 标题区域高度

	canvasHeight := fg.Height
	if canvasHeight == 0 {
		canvasHeight = titleHeight + padTop + (maxDepth+1)*frameHeight + padBottom
	}

	// 4. 生成 SVG 内容
	fg.writeSVG(output, root, totalSamples, maxDepth, frameHeight, padTop, padBottom, titleHeight, canvasHeight)

	return nil
}

// ==================== 树构建 ====================

// buildTree 从栈采样数据构建火焰图树
// 栈帧链 "main;foo;bar" 被解析为树形结构:
//
//	root (all)
//	└── main
//	    └── foo
//	        └── bar
func (fg *FlameGraph) buildTree(stackCounts map[string]uint64) *flameNode {
	root := &flameNode{
		name:     "all",
		value:    0,
		total:    0,
		children: make([]*flameNode, 0),
	}

	for stack, count := range stackCounts {
		// 将栈帧链分割为各个函数名
		frames := strings.Split(stack, ";")
		if len(frames) == 0 {
			continue
		}

		// 从根节点开始，逐级构建树
		current := root
		for _, frame := range frames {
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
				newNode := &flameNode{
					name:     frame,
					value:    0,
					total:    0,
					children: make([]*flameNode, 0),
				}
				current.children = append(current.children, newNode)
				current = newNode
			}
		}

		// 叶节点增加采样计数
		current.value += count
	}

	// 自底向上计算每个节点的总采样数
	fg.calculateTotals(root)

	return root
}

// calculateTotals 递归计算每个节点的总采样数
// total = 自身采样数 + 所有子节点采样数之和
func (fg *FlameGraph) calculateTotals(node *flameNode) uint64 {
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
// 用于确定火焰图的高度
func (fg *FlameGraph) calculateMaxDepth(node *flameNode) int {
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

// ==================== SVG 生成 ====================

// writeSVG 生成完整的 SVG 火焰图
// 包含: SVG 头部、样式定义、标题、栈帧矩形、搜索功能、底部信息
func (fg *FlameGraph) writeSVG(output io.Writer, root *flameNode, totalSamples uint64,
	maxDepth int, frameHeight, padTop, padBottom, titleHeight, canvasHeight int) {

	// 计算可用绘图宽度
	drawWidth := fg.Width - 20 // 左右各留 10 像素边距

	// 写入 SVG 头部和样式
	fmt.Fprintf(output, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, fg.Width, canvasHeight, fg.Width, canvasHeight)
	fmt.Fprintln(output)

	// 写入 CSS 样式
	fg.writeStyles(output)

	// 写入 JavaScript (搜索高亮功能)
	fg.writeJavaScript(output, totalSamples)

	// 写入标题
	fmt.Fprintf(output, `<text x="%d" y="20" text-anchor="middle" font-family="%s" font-size="16" fill="rgb(0,0,0)">%s</text>`,
		fg.Width/2, fg.FontFamily, fg.Title)
	fmt.Fprintln(output)

	// 写入搜索框
	fmt.Fprintf(output, `<text x="10" y="%d" font-family="%s" font-size="12" fill="rgb(0,0,0)">搜索: </text>`,
		canvasHeight-padBottom+15, fg.FontFamily)
	fmt.Fprintf(output, `<input id="search" type="text" oninput="searchFlamegraph(this.value)" style="font-size:12px" />`)
	fmt.Fprintln(output)

	// 写入统计信息
	fmt.Fprintf(output, `<text x="10" y="%d" font-family="%s" font-size="11" fill="rgb(100,100,100)">总采样数: %d</text>`,
		canvasHeight-padBottom+30, fg.FontFamily, totalSamples)
	fmt.Fprintln(output)

	// 写入火焰图主体 (递归绘制栈帧)
	fmt.Fprintf(output, `<g class="flamegraph">`)
	fmt.Fprintln(output)
	fg.writeFlameRects(output, root, 10, titleHeight+padTop, drawWidth, totalSamples, frameHeight, 0)
	fmt.Fprintf(output, `</g>`)
	fmt.Fprintln(output)

	// 写入 SVG 结束标签
	fmt.Fprintf(output, `</svg>`)
}

// writeStyles 写入 SVG 的 CSS 样式
// 包含: 矩形样式、悬停效果、搜索高亮样式
func (fg *FlameGraph) writeStyles(output io.Writer) {
	fmt.Fprintln(output, `<style>`)
	// 矩形默认样式
	fmt.Fprintln(output, `  .flamegraph rect { cursor: pointer; stroke: rgb(0,0,0); stroke-width: 0.5; }`)
	// 鼠标悬停效果: 高亮边框
	fmt.Fprintln(output, `  .flamegraph rect:hover { stroke: rgb(0,0,255); stroke-width: 1.5; }`)
	// 搜索高亮样式: 匹配的矩形保持原色，不匹配的变灰
	fmt.Fprintln(output, `  .flamegraph rect.dimmed { opacity: 0.3; }`)
	fmt.Fprintln(output, `  .flamegraph rect.highlighted { stroke: rgb(0,0,255); stroke-width: 2; }`)
	// 文字样式
	fmt.Fprintln(output, `  .flamegraph text { pointer-events: none; }`)
	fmt.Fprintln(output, `</style>`)
}

// writeJavaScript 写入搜索高亮的 JavaScript 代码
// 功能: 根据搜索关键词高亮匹配的栈帧，其余变暗
func (fg *FlameGraph) writeJavaScript(output io.Writer, totalSamples uint64) {
	fmt.Fprintln(output, `<script type="text/ecmascript"><![CDATA[`)

	// searchFlamegraph 函数: 搜索并高亮匹配的栈帧
	fmt.Fprintln(output, `  function searchFlamegraph(query) {`)
	fmt.Fprintln(output, `    var rects = document.querySelectorAll('.flamegraph rect');`)
	fmt.Fprintln(output, `    var texts = document.querySelectorAll('.flamegraph text');`)
	fmt.Fprintln(output, `    if (query === '') {`)
	fmt.Fprintln(output, `      // 清空搜索时恢复所有矩形`)
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

// writeFlameRects 递归绘制火焰图的栈帧矩形
// 参数:
//   - output: SVG 输出流
//   - node: 当前树节点
//   - x: 矩形左上角 X 坐标
//   - y: 矩形左上角 Y 坐标
//   - width: 矩形宽度 (像素)
//   - totalSamples: 总采样数 (用于计算百分比)
//   - frameHeight: 栈帧高度 (像素)
//   - depth: 当前深度 (用于颜色变化)
func (fg *FlameGraph) writeFlameRects(output io.Writer, node *flameNode, x, y, width int,
	totalSamples uint64, frameHeight, depth int) {

	// 如果宽度小于最小宽度，不绘制 (避免过小的矩形)
	if float64(width) < fg.MinWidth {
		return
	}

	// 计算百分比
	percentage := float64(node.total) * 100.0 / float64(totalSamples)

	// 生成基于函数名的颜色 (暖色调: 红->橙->黄)
	color := fg.generateColor(node.name, depth)

	// 计算采样百分比字符串
	pctStr := fmt.Sprintf("%.2f%%", percentage)

	// 生成悬停提示信息 (title 属性)
	title := fmt.Sprintf("%s (%d 采样, %s)", node.name, node.total, pctStr)

	// 写入矩形元素
	fmt.Fprintf(output, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" title="%s"/>`,
		x, y, width, frameHeight, color, title)
	fmt.Fprintln(output)

	// 如果矩形足够宽，写入函数名文字
	// 文字只在宽度足够时显示，避免重叠
	if width > 30 {
		// 截断过长的函数名以适应矩形宽度
		displayName := fg.truncateText(node.name, width-6)
		fmt.Fprintf(output, `<text x="%d" y="%d" font-family="%s" font-size="%d" fill="%s">%s</text>`,
			x+3, y+frameHeight-3, fg.FontFamily, fg.FontSize, fg.FontColor, displayName)
		fmt.Fprintln(output)
	}

	// 递归绘制子节点
	childX := x
	for _, child := range node.children {
		// 计算子节点宽度 (按采样比例)
		childWidth := float64(child.total) * float64(width) / float64(node.total)
		childWidthInt := int(childWidth)

		if childWidthInt > 0 {
			fg.writeFlameRects(output, child, childX, y+frameHeight, childWidthInt,
				totalSamples, frameHeight, depth+1)
			childX += childWidthInt
		}
	}
}

// ==================== 颜色生成 ====================

// generateColor 基于函数名和深度生成暖色调颜色
// 颜色范围: 红色 -> 橙色 -> 黄色
// 使用函数名的 hash 值确定色相，深度影响亮度
func (fg *FlameGraph) generateColor(name string, depth int) string {
	// 使用 FNV-1a hash 计算函数名的哈希值
	h := fnv.New32a()
	h.Write([]byte(name))
	hash := h.Sum32()

	// 将 hash 映射到暖色调范围
	// 暖色调 HSL 色相范围: 0 (红) -> 60 (黄)
	// 跳过 40-50 (黄绿色) 以保持暖色调

	// 色相: 0-60 范围 (红->橙->黄)
	hue := float64(hash%60) + 0 // 0-60

	// 饱和度: 60%-100%，基于 hash 变化
	saturation := 60.0 + float64((hash>>8)%40)

	// 亮度: 50%-75%，基于深度微调
	// 深度越深，亮度略低，形成层次感
	lightness := 70.0 - float64(depth%5)*3.0
	if lightness < 50 {
		lightness = 50
	}

	return fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", hue, saturation, lightness)
}

// ==================== 文本处理 ====================

// truncateText 截断文本以适应指定宽度
// 根据字符宽度估算 (中文字符约等于 2 个英文字符宽度)
func (fg *FlameGraph) truncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	// 估算每个字符的平均宽度 (基于字体大小)
	// 英文字符约 0.6 * fontSize，中文字符约 1.0 * fontSize
	charWidth := float64(fg.FontSize) * 0.6
	maxChars := int(float64(maxWidth) / charWidth)

	if len(text) <= maxChars {
		return text
	}

	// 截断并添加省略号
	if maxChars > 3 {
		return text[:maxChars-3] + "..."
	}
	return text[:maxChars]
}

// ==================== 热点函数分析 ====================

// GenerateHotFunctions 分析栈采样数据，返回 TopN 热点函数列表
// 热点函数按采样次数降序排列
// 参数:
//   - stackCounts: 栈合并计数 map
//   - topN: 返回前 N 个热点函数
//
// 返回: 热点函数列表，按采样次数降序排列
func (fg *FlameGraph) GenerateHotFunctions(stackCounts map[string]uint64, topN int) []HotFunction {
	if len(stackCounts) == 0 {
		return nil
	}

	// 1. 统计每个函数的总采样数
	// 一个函数可能在多个栈中出现，需要累加
	funcSamples := make(map[string]uint64)
	totalSamples := uint64(0)

	for stack, count := range stackCounts {
		totalSamples += count
		frames := strings.Split(stack, ";")
		for _, frame := range frames {
			if frame != "" {
				funcSamples[frame] += count
			}
		}
	}

	// 2. 转换为 HotFunction 列表
	var hotFuncs []HotFunction
	for name, samples := range funcSamples {
		percentage := float64(samples) * 100.0 / float64(totalSamples)
		hotFuncs = append(hotFuncs, HotFunction{
			Name:       name,
			Samples:    samples,
			Percentage: percentage,
		})
	}

	// 3. 按采样次数降序排序
	sort.Slice(hotFuncs, func(i, j int) bool {
		if hotFuncs[i].Samples != hotFuncs[j].Samples {
			return hotFuncs[i].Samples > hotFuncs[j].Samples
		}
		// 采样数相同时，按函数名字典序排列
		return hotFuncs[i].Name < hotFuncs[j].Name
	})

	// 4. 返回前 topN 个
	if topN > 0 && len(hotFuncs) > topN {
		return hotFuncs[:topN]
	}

	return hotFuncs
}
