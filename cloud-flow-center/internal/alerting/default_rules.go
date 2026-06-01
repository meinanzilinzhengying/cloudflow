// Package alerting 提供告警规则引擎和通知功能
package alerting

import (
	"time"
)

// DefaultRules 返回默认的告警规则
func DefaultRules() []*Rule {
	// 捕获一次时间戳，确保同一批创建的规则时间戳一致
	now := time.Now()
	return []*Rule{
		{
			ID:          "rule-cpu-high",
			Name:        "CPU 使用率过高",
			Description: "当 CPU 使用率超过 80% 时触发告警",
			Type:        RuleTypeCPU,
			Enabled:     true,
			Condition: Condition{
				Metric:    "cpu_usage",
				Operator:  OperatorGreaterThan,
				Threshold: 80.0,
			},
			Threshold: 80.0,
			Duration:  Duration{Duration: 2 * time.Minute},
			Severity:  "warning",
			Labels: map[string]string{
				"category": "system",
				"service":  "cloud-flow",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "rule-memory-high",
			Name:        "内存使用率过高",
			Description: "当内存使用率超过 85% 时触发告警",
			Type:        RuleTypeMemory,
			Enabled:     true,
			Condition: Condition{
				Metric:    "memory_usage",
				Operator:  OperatorGreaterThan,
				Threshold: 85.0,
			},
			Threshold: 85.0,
			Duration:  Duration{Duration: 2 * time.Minute},
			Severity:  "warning",
			Labels: map[string]string{
				"category": "system",
				"service":  "cloud-flow",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "rule-network-high",
			Name:        "网络流量异常",
			Description: "当网络流量超过 1GB/s 时触发告警",
			Type:        RuleTypeNetwork,
			Enabled:     true,
			Condition: Condition{
				Metric:    "network_bytes",
				Operator:  OperatorGreaterThan,
				Threshold: 1073741824.0, // 1GB
			},
			Threshold: 1073741824.0,
			Duration:  Duration{Duration: 1 * time.Minute},
			Severity:  "info",
			Labels: map[string]string{
				"category": "network",
				"service":  "cloud-flow",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "rule-disk-high",
			Name:        "磁盘 IO 异常",
			Description: "当磁盘 IO 操作次数超过 10000/s 时触发告警",
			Type:        RuleTypeDisk,
			Enabled:     true,
			Condition: Condition{
				Metric:    "disk_ops",
				Operator:  OperatorGreaterThan,
				Threshold: 10000.0,
			},
			Threshold: 10000.0,
			Duration:  Duration{Duration: 1 * time.Minute},
			Severity:  "info",
			Labels: map[string]string{
				"category": "storage",
				"service":  "cloud-flow",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "rule-traffic-anomaly",
			Name:        "流量异常",
			Description: "当流量超过 500MB/s 时触发告警",
			Type:        RuleTypeTraffic,
			Enabled:     true,
			Condition: Condition{
				Metric:    "traffic_bytes",
				Operator:  OperatorGreaterThan,
				Threshold: 536870912.0, // 500MB
			},
			Threshold: 536870912.0,
			Duration:  Duration{Duration: 1 * time.Minute},
			Severity:  "info",
			Labels: map[string]string{
				"category": "network",
				"service":  "cloud-flow",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}
