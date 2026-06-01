package storage

import "fmt"

// MetricNameToColumn 将指标名称映射到数据库列名
func MetricNameToColumn(metricName string) (string, error) {
	validColumns := map[string]string{
		"bytes":        "bytes",
		"packets":      "packets",
		"latency":      "latency",
		"cpu_usage":    "cpu_usage",
		"memory_usage": "memory_usage",
		"disk_usage":   "disk_usage",
	}
	col, ok := validColumns[metricName]
	if !ok {
		return "", fmt.Errorf("无效的指标名称: %s", metricName)
	}
	return col, nil
}
