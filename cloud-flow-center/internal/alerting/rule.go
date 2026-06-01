// Package alerting 提供告警规则引擎和通知功能
package alerting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cloud-flow-center/pkg/logger"
)

// Duration 对 time.Duration 的封装，支持 JSON 序列化/反序列化
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	case float64:
		d.Duration = time.Duration(value)
	default:
		return fmt.Errorf("invalid duration type")
	}
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

type RuleType string

const (
	RuleTypeCPU     RuleType = "cpu"
	RuleTypeMemory  RuleType = "memory"
	RuleTypeNetwork RuleType = "network"
	RuleTypeDisk    RuleType = "disk"
	RuleTypeTraffic RuleType = "traffic"
)

type ConditionOperator string

const (
	OperatorGreaterThan    ConditionOperator = ">"
	OperatorLessThan       ConditionOperator = "<"
	OperatorGreaterOrEqual ConditionOperator = ">="
	OperatorLessOrEqual    ConditionOperator = "<="
	OperatorEqual          ConditionOperator = "="
	OperatorNotEqual       ConditionOperator = "!="
)

// Rule 告警规则
type Rule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Type             RuleType          `json:"type"`
	Enabled          bool              `json:"enabled"`
	Condition        Condition         `json:"condition"`
	Threshold        float64           `json:"threshold"`
	Duration         Duration          `json:"duration"`
	Severity         string            `json:"severity"`
	Labels           map[string]string `json:"labels"`
	SatisfyThreshold float64           `json:"satisfy_threshold"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// Condition 规则条件
type Condition struct {
	Metric    string            `json:"metric"`
	Operator  ConditionOperator `json:"operator"`
	Threshold float64           `json:"threshold"`
}

// RuleManager 规则管理器
type RuleManager struct {
	mu       sync.RWMutex
	rules    map[string]*Rule
	ruleDir  string
	logger   *logger.Logger
	stopped  bool
}

// NewRuleManager 创建规则管理器
func NewRuleManager(ruleDir string, log *logger.Logger) *RuleManager {
	return &RuleManager{
		rules:   make(map[string]*Rule),
		ruleDir: ruleDir,
		logger:  log,
	}
}

// LoadRules 从目录加载规则
func (rm *RuleManager) LoadRules() error {
	if err := os.MkdirAll(rm.ruleDir, 0755); err != nil {
		return fmt.Errorf("创建规则目录失败: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(rm.ruleDir, "*.json"))
	if err != nil {
		return fmt.Errorf("读取规则目录失败: %w", err)
	}

	if len(files) == 0 {
		rm.logger.Info("未找到规则文件，创建默认规则")
		rm.createDefaultRules()
		return nil
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			rm.logger.Warnf("读取规则文件失败 %s: %v", file, err)
			continue
		}

		var rule Rule
		if err := json.Unmarshal(data, &rule); err != nil {
			rm.logger.Warnf("解析规则文件失败 %s: %v", file, err)
			continue
		}

		if rule.ID == "" {
			rm.logger.Warnf("规则文件 %s 缺少 ID，跳过", file)
			continue
		}

		rm.rules[rule.ID] = &rule
	}

	rm.logger.Infof("加载了 %d 条规则", len(rm.rules))
	return nil
}

// createDefaultRules 创建默认规则
func (rm *RuleManager) createDefaultRules() {
	defaultRules := []Rule{
		{
			ID:        "rule-cpu-high",
			Name:      "CPU 使用率过高",
			Type:      RuleTypeCPU,
			Enabled:   true,
			Condition: Condition{Operator: OperatorGreaterThan, Threshold: 80},
			Threshold: 80,
			Duration:  Duration{5 * time.Minute},
			Severity:  "warning",
			Labels:    map[string]string{"type": "cpu"},
		},
		{
			ID:        "rule-memory-high",
			Name:      "内存使用率过高",
			Type:      RuleTypeMemory,
			Enabled:   true,
			Condition: Condition{Operator: OperatorGreaterThan, Threshold: 90},
			Threshold: 90,
			Duration:  Duration{5 * time.Minute},
			Severity:  "critical",
			Labels:    map[string]string{"type": "memory"},
		},
	}

	for i := range defaultRules {
		rule := &defaultRules[i]
		rm.rules[rule.ID] = rule
		if err := rm.SaveRule(rule); err != nil {
			rm.logger.Warnf("保存默认规则失败 %s: %v", rule.ID, err)
		}
	}
	rm.logger.Infof("创建了 %d 条默认规则", len(defaultRules))
}

// SaveRule 保存规则到文件
func (rm *RuleManager) SaveRule(rule *Rule) error {
	rm.mu.Lock()
	rm.rules[rule.ID] = rule
	rm.mu.Unlock()

	data, err := json.MarshalIndent(rule, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化规则失败: %w", err)
	}

	ruleFile := filepath.Join(rm.ruleDir, rule.ID+".json")
	if err := os.WriteFile(ruleFile, data, 0644); err != nil {
		return fmt.Errorf("写入规则文件失败: %w", err)
	}

	return nil
}

// GetRules 获取所有规则
func (rm *RuleManager) GetRules() []*Rule {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rules := make([]*Rule, 0, len(rm.rules))
	for _, rule := range rm.rules {
		rules = append(rules, rule)
	}
	return rules
}

// GetRuleByID 根据 ID 获取规则
func (rm *RuleManager) GetRuleByID(id string) *Rule {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rule, ok := rm.rules[id]
	if !ok {
		return nil
	}
	return rule
}

// DeleteRule 删除规则
func (rm *RuleManager) DeleteRule(id string) error {
	rm.mu.Lock()
	delete(rm.rules, id)
	rm.mu.Unlock()

	ruleFile := filepath.Join(rm.ruleDir, id+".json")
	if err := os.Remove(ruleFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除规则文件失败: %w", err)
	}

	return nil
}
