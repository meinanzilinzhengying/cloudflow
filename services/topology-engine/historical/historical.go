// Package historical 历史拓扑查询
//
// 从 ClickHouse topology 表查询历史拓扑数据:
//   - 按时间范围查询
//   - 按租户过滤
//   - 按图类型聚合
//   - 支持时间桶聚合 (用于趋势分析)
package historical

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	graph "cloud-flow/services/topology-engine/graph"
	svcproto "cloud-flow/services/proto"
)

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

// ClickHouseConfig ClickHouse 连接配置
type ClickHouseConfig struct {
	// Addr ClickHouse HTTP 接口地址，例如 "http://localhost:8123"
	Addr string
	// Database 数据库名称，默认 "cloudflow"
	Database string
	// User 用户名，默认 "default"
	User string
	// Password 密码
	Password string
	// Timeout HTTP 请求超时，默认 30s
	Timeout time.Duration
	// MaxRows 单次查询最大返回行数，默认 1000000
	MaxRows int
}

// applyDefaults 对零值字段填充默认值。
func (c *ClickHouseConfig) applyDefaults() {
	if c.Database == "" {
		c.Database = "cloudflow"
	}
	if c.User == "" {
		c.User = "default"
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxRows == 0 {
		c.MaxRows = 1_000_000
	}
}

// ---------------------------------------------------------------------------
// HistoricalQuery
// ---------------------------------------------------------------------------

// HistoricalQuery 历史拓扑查询器，通过 ClickHouse HTTP 接口查询历史拓扑数据
type HistoricalQuery struct {
	config     ClickHouseConfig
	httpClient *http.Client
}

// NewHistoricalQuery 创建历史拓扑查询器
func NewHistoricalQuery(config ClickHouseConfig) *HistoricalQuery {
	config.applyDefaults()
	return &HistoricalQuery{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// ---------------------------------------------------------------------------
// 公开查询方法
// ---------------------------------------------------------------------------

// QueryTopology 按时间范围查询历史拓扑，构建 graph.Graph 并返回。
//
// graphType 可选值: "service" / "pod" / "namespace" / "process"
func (q *HistoricalQuery) QueryTopology(
	ctx context.Context,
	tenantID string,
	startTime, endTime int64,
	graphType string,
) (*graph.Graph, error) {
	srcCol, dstCol, err := resolveGraphColumns(graphType)
	if err != nil {
		return nil, err
	}

	sql := fmt.Sprintf(
		`SELECT %[1]s AS src, %[2]s AS dst, protocol, `+
			`sum(bytes) AS bytes, sum(packets) AS packets, `+
			`avg(latency) AS latency, sum(errors) AS errors `+
			`FROM topology `+
			`WHERE tenant_id = '%[3]s' AND timestamp BETWEEN %[4]d AND %[5]d `+
			`GROUP BY %[1]s, %[2]s, protocol `+
			`LIMIT %[6]d`,
		srcCol, dstCol,
		escapeSQLString(tenantID),
		startTime, endTime,
		q.config.MaxRows,
	)

	resp, err := q.executeQuery(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query topology failed: %w", err)
	}

	g := parseGraphFromRows(resp.Data, graphType, tenantID)
	return g, nil
}

// QueryHeatmap 查询热力图数据，按时间桶聚合。
//
// metric 可选值: "latency" / "error_rate"
// interval 为时间桶大小（秒）
func (q *HistoricalQuery) QueryHeatmap(
	ctx context.Context,
	tenantID string,
	startTime, endTime int64,
	metric string,
	interval int64,
) ([]*svcproto.HeatmapPoint, error) {
	valueExpr, err := resolveMetricExpr(metric)
	if err != nil {
		return nil, err
	}

	sql := fmt.Sprintf(
		`SELECT `+
			`toStartOfInterval(timestamp, INTERVAL %[1]d SECOND) AS bucket, `+
			`src_service AS src_id, dst_service AS dst_id, `+
			`%[2]s AS value, count() AS count `+
			`FROM topology `+
			`WHERE tenant_id = '%[3]s' AND timestamp BETWEEN %[4]d AND %[5]d `+
			`GROUP BY bucket, src_service, dst_service `+
			`ORDER BY bucket `+
			`LIMIT %[6]d`,
		interval,
		valueExpr,
		escapeSQLString(tenantID),
		startTime, endTime,
		q.config.MaxRows,
	)

	resp, err := q.executeQuery(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query heatmap failed: %w", err)
	}

	points := make([]*svcproto.HeatmapPoint, 0, len(resp.Data))
	for _, row := range resp.Data {
		p := &svcproto.HeatmapPoint{
			Source:    getStringField(row, "src_id"),
			Target:    getStringField(row, "dst_id"),
			Timestamp: getInt64Field(row, "bucket"),
			Value:     getFloat64Field(row, "value"),
			Count:     getUint64Field(row, "count"),
		}
		points = append(points, p)
	}

	return points, nil
}

// QueryTopologyDiff 查询两个时间点的拓扑差异。
//
// baseTime 和 compareTime 分别为基准时间和对比时间（UnixNano）。
// 查询时会以该时间点前后 5 分钟作为查询窗口。
func (q *HistoricalQuery) QueryTopologyDiff(
	ctx context.Context,
	tenantID string,
	baseTime, compareTime int64,
	graphType string,
) (*graph.TopologyDiff, error) {
	// 查询窗口: 时间点前后 5 分钟
	window := int64(5 * 60 * 1e9) // 5 分钟 (纳秒)

	baseGraph, err := q.QueryTopology(
		ctx, tenantID,
		baseTime-window, baseTime+window,
		graphType,
	)
	if err != nil {
		return nil, fmt.Errorf("query base topology failed: %w", err)
	}

	compareGraph, err := q.QueryTopology(
		ctx, tenantID,
		compareTime-window, compareTime+window,
		graphType,
	)
	if err != nil {
		return nil, fmt.Errorf("query compare topology failed: %w", err)
	}

	diff := baseGraph.Diff(compareGraph)
	return diff, nil
}

// ---------------------------------------------------------------------------
// ClickHouse HTTP 交互
// ---------------------------------------------------------------------------

// clickHouseResponse ClickHouse HTTP 接口返回的 JSON 结构
type clickHouseResponse struct {
	Data []map[string]interface{} `json:"data"`
	Rows int                      `json:"rows"`
	Meta []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"meta"`
}

// executeQuery 通过 ClickHouse HTTP 接口执行 SQL 查询
func (q *HistoricalQuery) executeQuery(ctx context.Context, sql string) (*clickHouseResponse, error) {
	u, err := url.Parse(q.config.Addr)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse addr: %w", err)
	}

	params := url.Values{}
	params.Set("database", q.config.Database)
	params.Set("user", q.config.User)
	if q.config.Password != "" {
		params.Set("password", q.config.Password)
	}
	// 使用 JSON 格式返回，便于解析
	params.Set("default_format", "JSON")

	u.Path = "/"
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader([]byte(sql)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	httpResp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024*1024)) // 限制 64MB
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clickhouse error (status %d): %s", httpResp.StatusCode, string(body))
	}

	var resp clickHouseResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// ---------------------------------------------------------------------------
// 图解析
// ---------------------------------------------------------------------------

// parseGraphFromRows 将 ClickHouse 查询结果行解析为 graph.Graph
func parseGraphFromRows(rows []map[string]interface{}, graphType string, tenantID string) *graph.Graph {
	g := graph.NewGraph(graphType, tenantID, graph.DefaultMaxNodes, graph.DefaultMaxEdges)

	for _, row := range rows {
		src := getStringField(row, "src")
		dst := getStringField(row, "dst")
		protocol := getStringField(row, "protocol")

		if src == "" || dst == "" {
			continue
		}

		// 添加节点
		g.AddOrUpdateNode(src, src, graphType, "", nil)
		g.AddOrUpdateNode(dst, dst, graphType, "", nil)

		// 添加边
		bytesVal := getUint64Field(row, "bytes")
		packetsVal := getUint64Field(row, "packets")
		latencyVal := getUint64Field(row, "latency")
		errorsVal := getUint64Field(row, "errors")

		g.AccumulateEdge(src, dst, bytesVal, packetsVal, latencyVal, errorsVal)

		// 设置边的协议（AccumulateEdge 不会设置 protocol）
		if e, ok := g.GetEdge(src, dst); ok && protocol != "" {
			e.Protocol = protocol
		}
	}

	return g
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// resolveGraphColumns 根据 graphType 返回 ClickHouse 表中对应的源/目标列名
func resolveGraphColumns(graphType string) (srcCol, dstCol string, err error) {
	switch graphType {
	case "service":
		return "src_service", "dst_service", nil
	case "pod":
		return "src_pod", "dst_pod", nil
	case "namespace":
		return "src_namespace", "dst_namespace", nil
	case "process":
		return "src_process", "dst_process", nil
	default:
		return "", "", fmt.Errorf("unsupported graph type: %s (valid: service, pod, namespace, process)", graphType)
	}
}

// resolveMetricExpr 根据 metric 名称返回 ClickHouse 聚合表达式
func resolveMetricExpr(metric string) (string, error) {
	switch metric {
	case "latency":
		return "avg(latency)", nil
	case "error_rate":
		return "if(count() > 0, sum(errors) / count(), 0)", nil
	default:
		return "", fmt.Errorf("unsupported metric: %s (valid: latency, error_rate)", metric)
	}
}

// escapeSQLString 对字符串进行基本的 SQL 转义，防止 SQL 注入
func escapeSQLString(s string) string {
	// 替换单引号为两个单引号
	var buf bytes.Buffer
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			buf.WriteString("''")
		} else if c == '\\' {
			buf.WriteString("\\\\")
		} else {
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

// getStringField 从 map 中安全读取 string 字段
func getStringField(row map[string]interface{}, key string) string {
	v, ok := row[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// getInt64Field 从 map 中安全读取 int64 字段
func getInt64Field(row map[string]interface{}, key string) int64 {
	v, ok := row[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case json.Number:
		n, _ := val.Int64()
		return n
	case float64:
		return int64(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	case int64:
		return val
	case int:
		return int64(val)
	default:
		return 0
	}
}

// getUint64Field 从 map 中安全读取 uint64 字段
func getUint64Field(row map[string]interface{}, key string) uint64 {
	v := getInt64Field(row, key)
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// getFloat64Field 从 map 中安全读取 float64 字段
func getFloat64Field(row map[string]interface{}, key string) float64 {
	v, ok := row[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case json.Number:
		f, _ := val.Float64()
		return f
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case int64:
		return float64(val)
	case int:
		return float64(val)
	default:
		return 0
	}
}
