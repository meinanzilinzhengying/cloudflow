package trace

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FlameNode 火焰图节点
type FlameNode struct {
	Name     string      `json:"name"`
	Value    int         `json:"value"`       // 采样次数
	Children []*FlameNode `json:"children,omitempty"`
}

// FlameGraph 火焰图
type FlameGraph struct {
	Name      string     `json:"name"`
	StartTime time.Time  `json:"start_time"`
	Duration  float64    `json:"duration_sec"`  // 采集时长(秒)
	SampleRate int       `json:"sample_rate"`    // 采样频率(Hz)
	TotalSamples int     `json:"total_samples"`  // 总采样数
	Root      *FlameNode `json:"root"`
	Format    string     `json:"format"`         // flamegraph/icicle/topdown
	Metadata  FlameMeta  `json:"metadata"`
}

// FlameMeta 火焰图元数据
type FlameMeta struct {
	PID         uint32 `json:"pid"`
	ProcessName string `json:"process_name"`
	Language    string `json:"language"`       // go/java/python/c/cpp
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	CPUCores    int    `json:"cpu_cores"`
}

// FlameGraphConfig 火焰图配置
type FlameGraphConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	SampleFreq    int           `yaml:"sample_freq" json:"sample_freq"`
	MaxStackDepth int           `yaml:"max_stack_depth" json:"max_stack_depth"`
	DurationSec   int           `yaml:"duration_sec" json:"duration_sec"`
	OutputDir     string        `yaml:"output_dir" json:"output_dir"`
	MaxStored     int           `yaml:"max_stored" json:"max_stored"` // 最大存储火焰图数
	MinSamples    int           `yaml:"min_samples" json:"min_samples"` // 最小采样数(过滤噪声)
}

// FlameGraphStore 火焰图存储
type FlameGraphStore struct {
	mu       sync.RWMutex
	graphs   map[string]*FlameGraph   // 按ID索引
	byTime   []*FlameGraph            // 按时间索引
	byPID    map[uint32][]*FlameGraph // 按PID索引
	maxSize  int
}

// FlameGraphGenerator 火焰图生成器
type FlameGraphGenerator struct {
	store  *FlameGraphStore
	config *FlameGraphConfig
}

// NewFlameGraphGenerator 创建火焰图生成器
func NewFlameGraphGenerator(config *FlameGraphConfig) *FlameGraphGenerator {
	if config == nil {
		config = DefaultFlameGraphConfig()
	}
	maxStored := config.MaxStored
	if maxStored <= 0 {
		maxStored = 100
	}

	return &FlameGraphGenerator{
		store: &FlameGraphStore{
			graphs: make(map[string]*FlameGraph),
			byTime: make([]*FlameGraph, 0),
			byPID:  make(map[uint32][]*FlameGraph),
			maxSize: maxStored,
		},
		config: config,
	}
}

// DefaultFlameGraphConfig 默认火焰图配置
func DefaultFlameGraphConfig() *FlameGraphConfig {
	return &FlameGraphConfig{
		Enabled:       true,
		SampleFreq:    99,
		MaxStackDepth: 127,
		DurationSec:   30,
		OutputDir:     "/var/log/cloud-flow-agent/flamegraph",
		MaxStored:     100,
		MinSamples:    5,
	}
}

// ParseStacks 从原始栈数据解析生成火焰图
// stacks 格式: 每行一个栈帧，空行分隔不同采样
// 例如: "main.main\nruntime.main\nruntime.goexit\n\nmain.main\nruntime.main\n"
func (g *FlameGraphGenerator) ParseStacks(rawStacks string, meta FlameMeta) *FlameGraph {
	stacks := parseRawStacks(rawStacks)
	if len(stacks) == 0 {
		return nil
	}

	root := buildFlameTree(stacks, g.config.MinSamples)

	id := fmt.Sprintf("flame-%d-%d", meta.PID, time.Now().UnixNano())
	duration := float64(len(stacks)) / float64(g.config.SampleFreq)

	graph := &FlameGraph{
		Name:         fmt.Sprintf("%s (PID %d)", meta.ProcessName, meta.PID),
		StartTime:    time.Now(),
		Duration:     math.Round(duration*100) / 100,
		SampleRate:   g.config.SampleFreq,
		TotalSamples: len(stacks),
		Root:         root,
		Format:       "flamegraph",
		Metadata:     meta,
	}

	g.store.Add(id, graph)
	return graph
}

// ParseFoldedStacks 从folded格式解析
// folded格式: "frame1;frame2;frame3 count"
func (g *FlameGraphGenerator) ParseFoldedStacks(folded string, meta FlameMeta) *FlameGraph {
	root := &FlameNode{Name: "root"}

	lines := strings.Split(strings.TrimSpace(folded), "\n")
	totalSamples := 0

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			continue
		}

		frames := strings.Split(parts[0], ";")
		count, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			continue
		}

		totalSamples += count
		current := root

		for _, frame := range frames {
			frame = strings.TrimSpace(frame)
			if frame == "" {
				continue
			}

			// 查找或创建子节点
			found := false
			for _, child := range current.Children {
				if child.Name == frame {
					current = child
					found = true
					break
				}
			}

			if !found {
				node := &FlameNode{Name: frame, Value: 0}
				current.Children = append(current.Children, node)
				current = node
			}
		}
		current.Value += count
	}

	// 合并相同节点
	mergeNodes(root)

	// 计算root值
	root.Value = totalSamples

	id := fmt.Sprintf("flame-%d-%d", meta.PID, time.Now().UnixNano())
	duration := float64(totalSamples) / float64(g.config.SampleFreq)

	graph := &FlameGraph{
		Name:         fmt.Sprintf("%s (PID %d)", meta.ProcessName, meta.PID),
		StartTime:    time.Now(),
		Duration:     math.Round(duration*100) / 100,
		SampleRate:   g.config.SampleFreq,
		TotalSamples: totalSamples,
		Root:         root,
		Format:       "flamegraph",
		Metadata:     meta,
	}

	g.store.Add(id, graph)
	return graph
}

// GetFlameGraph 获取火焰图
func (g *FlameGraphGenerator) GetFlameGraph(id string) (*FlameGraph, error) {
	return g.store.Get(id)
}

// ListFlameGraphs 列出火焰图
func (g *FlameGraphGenerator) ListFlameGraphs(pid uint32, limit int) []*FlameGraph {
	return g.store.List(pid, limit)
}

// GetFlameGraphSVG 生成SVG火焰图
func (g *FlameGraphGenerator) GetFlameGraphSVG(id string) (string, error) {
	graph, err := g.store.Get(id)
	if err != nil {
		return "", err
	}
	return renderFlameSVG(graph), nil
}

// GetFlameGraphHTML 生成HTML火焰图页面
func (g *FlameGraphGenerator) GetFlameGraphHTML(id string) (string, error) {
	graph, err := g.store.Get(id)
	if err != nil {
		return "", err
	}
	return renderFlameHTML(graph), nil
}

// GetTopFunctions 获取热点函数
func (g *FlameGraphGenerator) GetTopFunctions(id string, topN int) []FunctionHotspot {
	graph, err := g.store.Get(id)
	if err != nil {
		return nil
	}
	return extractTopFunctions(graph.Root, topN)
}

// FunctionHotspot 热点函数
type FunctionHotspot struct {
	Name      string  `json:"name"`
	Samples   int     `json:"samples"`
	Percentage float64 `json:"percentage"`
}

// --- FlameGraphStore ---

func (s *FlameGraphStore) Add(id string, graph *FlameGraph) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.graphs[id] = graph
	s.byTime = append(s.byTime, graph)

	if graph.Metadata.PID > 0 {
		s.byPID[graph.Metadata.PID] = append(s.byPID[graph.Metadata.PID], graph)
	}

	// 限制大小
	if len(s.byTime) > s.maxSize {
		removed := s.byTime[0]
		s.byTime = s.byTime[1:]
		delete(s.graphs, removed.Name)
	}
}

func (s *FlameGraphStore) Get(id string) (*FlameGraph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	graph, ok := s.graphs[id]
	if !ok {
		return nil, fmt.Errorf("flamegraph not found: %s", id)
	}
	return graph, nil
}

func (s *FlameGraphStore) List(pid uint32, limit int) []*FlameGraph {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var graphs []*FlameGraph
	if pid > 0 {
		graphs = s.byPID[pid]
	} else {
		graphs = s.byTime
	}

	// 按时间倒序
	result := make([]*FlameGraph, len(graphs))
	copy(result, graphs)
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}

	return result
}

// --- 内部函数 ---

// parseRawStacks 解析原始栈数据
func parseRawStacks(raw string) [][]string {
	// 按空行分隔不同采样
	blocks := strings.Split(raw, "\n\n")
	stacks := make([][]string, 0, len(blocks))

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		frames := strings.Split(block, "\n")
		cleaned := make([]string, 0, len(frames))
		for _, f := range frames {
			f = strings.TrimSpace(f)
			if f != "" {
				cleaned = append(cleaned, f)
			}
		}
		if len(cleaned) > 0 {
			stacks = append(stacks, cleaned)
		}
	}

	return stacks
}

// buildFlameTree 从栈数组构建火焰图树
func buildFlameTree(stacks [][]string, minSamples int) *FlameNode {
	root := &FlameNode{Name: "root"}

	for _, stack := range stacks {
		current := root
		for _, frame := range stack {
			frame = cleanFrameName(frame)
			found := false
			for _, child := range current.Children {
				if child.Name == frame {
					current = child
					found = true
					break
				}
			}
			if !found {
				node := &FlameNode{Name: frame}
				current.Children = append(current.Children, node)
				current = node
			}
		}
		current.Value++
	}

	// 过滤噪声节点
	if minSamples > 0 {
		filterNoise(root, minSamples)
	}

	// 计算root值
	root.Value = len(stacks)

	return root
}

// cleanFrameName 清理帧名称
func cleanFrameName(frame string) string {
	// 去掉地址前缀，如 0x1234 main.main → main.main
	re := regexp.MustCompile(`^0x[0-9a-f]+\s+`)
	frame = re.ReplaceAllString(frame, "")

	// 去掉偏移量，如 main.main+0x123 → main.main
	re = regexp.MustCompile(`\+0x[0-9a-f]+$`)
	frame = re.ReplaceAllString(frame, "")

	return strings.TrimSpace(frame)
}

// filterNoise 过滤采样数过低的节点
func filterNoise(node *FlameNode, min int) {
	if node == nil {
		return
	}

	var filtered []*FlameNode
	for _, child := range node.Children {
		filterNoise(child, min)
		if child.Value >= min || len(child.Children) > 0 {
			filtered = append(filtered, child)
		}
	}
	node.Children = filtered
}

// mergeNodes 合并同名子节点
func mergeNodes(node *FlameNode) {
	if node == nil {
		return
	}

	// 递归合并子节点
	for _, child := range node.Children {
		mergeNodes(child)
	}

	// 合并同名节点
	merged := make(map[string]*FlameNode)
	var order []string

	for _, child := range node.Children {
		if existing, ok := merged[child.Name]; ok {
			existing.Value += child.Value
			existing.Children = append(existing.Children, child.Children...)
		} else {
			copy := *child
			merged[child.Name] = &copy
			order = append(order, child.Name)
		}
	}

	result := make([]*FlameNode, 0, len(order))
	for _, name := range order {
		result = append(result, merged[name])
	}
	node.Children = result
}

// extractTopFunctions 提取热点函数
func extractTopFunctions(root *FlameNode, topN int) []FunctionHotspot {
	funcMap := make(map[string]int)
	collectFunctions(root, funcMap)

	type kv struct {
		Name    string
		Samples int
	}

	var sorted []kv
	for name, samples := range funcMap {
		sorted = append(sorted, kv{Name: name, Samples: samples})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Samples > sorted[j].Samples
	})

	total := root.Value
	if topN > 0 && topN < len(sorted) {
		sorted = sorted[:topN]
	}

	result := make([]FunctionHotspot, len(sorted))
	for i, kv := range sorted {
		pct := float64(kv.Samples) / float64(total) * 100
		result[i] = FunctionHotspot{
			Name:      kv.Name,
			Samples:   kv.Samples,
			Percentage: math.Round(pct*100) / 100,
		}
	}

	return result
}

// collectFunctions 递归收集所有函数
func collectFunctions(node *FlameNode, funcMap map[string]int) {
	if node == nil {
		return
	}
	if node.Name != "root" {
		funcMap[node.Name] += node.Value
	}
	for _, child := range node.Children {
		collectFunctions(child, funcMap)
	}
}

// --- SVG/HTML 渲染 ---

// renderFlameSVG 渲染SVG火焰图
func renderFlameSVG(graph *FlameGraph) string {
	var sb strings.Builder

	width := 1200
	height := 600
	minWidth := 1

	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, width, height, width, height))
	sb.WriteString(fmt.Sprintf(`<style>rect:hover{stroke:#000;stroke-width:1}text{font-size:11px;font-family:monospace;pointer-events:none}</style>`))
	sb.WriteString(fmt.Sprintf(`<title>%s - %d samples</title>`, graph.Name, graph.TotalSamples))

	// 渲染节点
	renderSVGLayer(&sb, graph.Root, 0, 0, width, 20, minWidth, height)

	sb.WriteString("</svg>")
	return sb.String()
}

func renderSVGLayer(sb *strings.Builder, node *FlameNode, x, y, totalWidth, rowHeight, minWidth, maxHeight int) {
	if node == nil || y >= maxHeight {
		return
	}

	if node.Value <= 0 {
		return
	}

	w := totalWidth
	if w < minWidth {
		w = minWidth
	}

	// 颜色基于函数名哈希
	color := flameColor(node.Name)

	sb.WriteString(fmt.Sprintf(`<g class="f"><title>%s (%d samples, %.1f%%)</title>`, node.Name, node.Value, 0.0))
	sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, x, y, w, rowHeight-1, color))

	// 文字(仅在宽度足够时显示)
	if w > 40 {
		text := truncateText(node.Name, (w-4)/7)
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d">%s</text>`, x+2, y+rowHeight-5, text))
	}
	sb.WriteString("</g>")

	// 渲染子节点
	childX := x
	for _, child := range node.Children {
		if node.Value <= 0 {
			continue
		}
		childWidth := int(float64(child.Value) / float64(node.Value) * float64(totalWidth))
		if childWidth < minWidth {
			continue
		}
		renderSVGLayer(sb, child, childX, y+rowHeight, childWidth, rowHeight, minWidth, maxHeight)
		childX += childWidth
	}
}

func flameColor(name string) string {
	// 基于名称生成暖色调颜色
	var hash uint32
	for _, c := range name {
		hash = hash*31 + uint32(c)
	}

	h := hash % 60
	s := 70 + (hash%30)
	l := 50 + (hash%20)

	return fmt.Sprintf("hsl(%d,%d%%,%d%%)", h, s, l)
}

func truncateText(text string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars-2] + ".."
}

// renderFlameHTML 渲染HTML火焰图页面
func renderFlameHTML(graph *FlameGraph) string {
	svgData := renderFlameSVG(graph)

	tmpl := template.Must(template.New("flame").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>{{.Name}} - Flame Graph</title>
<style>
body{margin:0;font-family:system-ui;background:#fff}
.header{padding:16px;background:#1a1a2e;color:#fff}
.header h1{margin:0;font-size:18px}
.header .meta{color:#aaa;font-size:13px;margin-top:4px}
.container{padding:16px}
svg{width:100%;height:auto}
</style>
</head>
<body>
<div class="header">
<h1>{{.Name}}</h1>
<div class="meta">
Samples: {{.TotalSamples}} | Duration: {{.Duration}}s | Rate: {{.SampleRate}}Hz |
PID: {{.Metadata.PID}} | Process: {{.Metadata.ProcessName}} | Language: {{.Metadata.Language}}
</div>
</div>
<div class="container">
{{.SVG}}
</div>
</body>
</html>`))

	var buf strings.Builder
	tmpl.Execute(&buf, struct {
		*FlameGraph
		SVG string
	}{
		FlameGraph: graph,
		SVG:        svgData,
	})

	return buf.String()
}

// --- FlameGraphAPI ---

// FlameGraphAPI 火焰图HTTP API
type FlameGraphAPI struct {
	generator *FlameGraphGenerator
}

// NewFlameGraphAPI 创建火焰图API
func NewFlameGraphAPI(generator *FlameGraphGenerator) *FlameGraphAPI {
	return &FlameGraphAPI{generator: generator}
}

// RegisterRoutes 注册路由
func (api *FlameGraphAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/flamegraph/list", api.handleList)
	mux.HandleFunc("/api/v1/flamegraph/", api.handleGet)
	mux.HandleFunc("/api/v1/flamegraph/", api.handleSVG)
	mux.HandleFunc("/api/v1/flamegraph/", api.handleHTML)
	mux.HandleFunc("/api/v1/flamegraph/hotspots", api.handleHotspots)
}

// handleList 列出火焰图
func (api *FlameGraphAPI) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var pid uint32
	if p := r.URL.Query().Get("pid"); p != "" {
		v, _ := strconv.ParseUint(p, 10, 32)
		pid = uint32(v)
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	graphs := api.generator.ListFlameGraphs(pid, limit)

	// 返回摘要列表
	type summary struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		StartTime    time.Time `json:"start_time"`
		Duration     float64 `json:"duration_sec"`
		TotalSamples int     `json:"total_samples"`
		PID          uint32  `json:"pid"`
		ProcessName  string  `json:"process_name"`
	}

	summaries := make([]summary, len(graphs))
	for i, g := range graphs {
		summaries[i] = summary{
			ID:           g.Name,
			Name:         g.Name,
			StartTime:    g.StartTime,
			Duration:     g.Duration,
			TotalSamples: g.TotalSamples,
			PID:          g.Metadata.PID,
			ProcessName:  g.Metadata.ProcessName,
		}
	}

	writeJSON(w, summaries)
}

// handleGet 获取火焰图JSON
func (api *FlameGraphAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/api/v1/flamegraph/")
	if id == "" {
		http.Error(w, "Missing flamegraph ID", http.StatusBadRequest)
		return
	}

	graph, err := api.generator.GetFlameGraph(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, graph)
}

// handleSVG 获取SVG
func (api *FlameGraphAPI) handleSVG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/api/v1/flamegraph/")
	if id == "" {
		http.Error(w, "Missing flamegraph ID", http.StatusBadRequest)
		return
	}

	svg, err := api.generator.GetFlameGraphSVG(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write([]byte(svg))
}

// handleHTML 获取HTML页面
func (api *FlameGraphAPI) handleHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/api/v1/flamegraph/")
	if id == "" {
		http.Error(w, "Missing flamegraph ID", http.StatusBadRequest)
		return
	}

	html, err := api.generator.GetFlameGraphHTML(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleHotspots 获取热点函数
func (api *FlameGraphAPI) handleHotspots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	topN := 20
	if t := r.URL.Query().Get("top"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v > 0 {
			topN = v
		}
	}

	hotspots := api.generator.GetTopFunctions(id, topN)
	if hotspots == nil {
		http.Error(w, "Flamegraph not found", http.StatusNotFound)
		return
	}

	writeJSON(w, hotspots)
}

func extractIDFromPath(path, prefix string) string {
	p := strings.TrimPrefix(path, prefix)
	// 去掉后缀
	if idx := strings.Index(p, "/"); idx > 0 {
		p = p[:idx]
	}
	return p
}
