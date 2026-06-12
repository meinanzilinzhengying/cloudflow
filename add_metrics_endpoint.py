import sys

with open('services/control-plane/service.go', 'r', encoding='utf-8') as f:
    lines = f.readlines()

# Find the line after collectServiceHealth (after the closing brace at line 873)
# Line 873 is "}" (closing brace of collectServiceHealth)
# Line 874 is "func (s *Service) listAgentsHandler..."

# Insert the new metricsHistoryHandler function before line 874
insert_idx = 873  # Insert after the closing brace of collectServiceHealth

new_func = '''
// metricsHistoryHandler 从 VictoriaMetrics 查询历史指标数据
// 查询参数: ?range=5m&interval=10s
// 返回: { cpu: [...], memory: [...], network_in: [...], network_out: [...], disk_used: [...], disk_free: [...] }
func (s *Service) metricsHistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rangeStr := r.URL.Query().Get("range")
	intervalStr := r.URL.Query().Get("interval")
	if rangeStr == "" {
		rangeStr = "5m"
	}
	if intervalStr == "" {
		intervalStr = "10s"
	}

	// 解析时间范围
	rangeSec := parseDuration(rangeStr)
	intervalSec := parseDuration(intervalStr)
	if rangeSec <= 0 {
		rangeSec = 300
	}
	if intervalSec <= 0 {
		intervalSec = 10
	}

	end := time.Now()
	start := end.Add(-time.Duration(rangeSec) * time.Second)

	// 查询 VictoriaMetrics
	vmAddr := "http://victoriametrics:8428"
	metrics := []string{"cpu_usage_percent", "memory_usage_percent"}
	
	result := make(map[string]interface{})
	for _, metric := range metrics {
		data := s.queryVictoriaMetrics(vmAddr, metric, start, end, intervalSec)
		result[metric] = data
	}

	json.NewEncoder(w).Encode(result)
}

// queryVictoriaMetrics 查询 VictoriaMetrics 时序数据
func (s *Service) queryVictoriaMetrics(addr, metric string, start, end time.Time, step int) []map[string]interface{} {
	queryURL := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=%d",
		addr, metric, start.Unix(), end.Unix(), step)
	
	resp, err := http.Get(queryURL)
	if err != nil {
		return []map[string]interface{}{}
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][]interface{} `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return []map[string]interface{}{}
	}

	// 转换为前端图表格式
	points := []map[string]interface{}{}
	for _, r := range result.Data.Result {
		for _, v := range r.Values {
			if len(v) >= 2 {
				ts := time.Unix(int64(v[0].(float64)), 0)
				val := v[1]
				points = append(points, map[string]interface{}{
					"timestamp": ts.Format("15:04"),
					"value":     val,
				})
			}
		}
	}

	return points
}

// parseDuration 解析时长字符串（如 "5m", "1h", "30s"）→ 秒数
func parseDuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num := 0
	for _, c := range numStr {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		}
	}
	switch unit {
	case 's', 'S':
		return num
	case 'm', 'M':
		return num * 60
	case 'h', 'H':
		return num * 3600
	case 'd', 'D':
		return num * 86400
	}
	return 0
}
'''

lines.insert(insert_idx, new_func)

with open('services/control-plane/service.go', 'w', encoding='utf-8') as f:
    f.writelines(lines)

print('OK: added metricsHistoryHandler')
