//go:build linux

package sysconfig

import (
	"encoding/json"
	"fmt"
	"time"
)

// ============================================================================
// 一、配置项模型
// ============================================================================

// ConfigValueType 配置值类型
type ConfigValueType string

const (
	ValueTypeString      ConfigValueType = "string"
	ValueTypeInt         ConfigValueType = "int"
	ValueTypeBool        ConfigValueType = "bool"
	ValueTypeFloat       ConfigValueType = "float"
	ValueTypeJSON        ConfigValueType = "json"
	ValueTypeYAML        ConfigValueType = "yaml"
	ValueTypeStringSlice ConfigValueType = "string_slice"
)

// ConfigItem 配置项
type ConfigItem struct {
	Key         string          `json:"key"`
	Value       interface{}     `json:"value"`
	Type        ConfigValueType `json:"type"`
	Description string          `json:"description"`
	Category    string          `json:"category"`  // 配置分类
	Default     interface{}     `json:"default"`   // 默认值
	Editable    bool            `json:"editable"`  // 是否可编辑
	Sensitive   bool            `json:"sensitive"` // 是否敏感（脱敏显示）
	Validation  *ValidationRule `json:"validation,omitempty"` // 验证规则
	Source      string          `json:"source"`    // 来源：file/env/remote

	// 元数据
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"` // 最后修改者
}

// StringValue 获取字符串值
func (ci *ConfigItem) StringValue() string {
	if ci.Value == nil {
		return ""
	}
	if s, ok := ci.Value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", ci.Value)
}

// IntValue 获取整数值
func (ci *ConfigItem) IntValue() int {
	switch v := ci.Value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// BoolValue 获取布尔值
func (ci *ConfigItem) BoolValue() bool {
	if b, ok := ci.Value.(bool); ok {
		return b
	}
	return ci.StringValue() == "true"
}

// FloatValue 获取浮点值
func (ci *ConfigItem) FloatValue() float64 {
	switch v := ci.Value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

// IsDefault 是否等于默认值
func (ci *ConfigItem) IsDefault() bool {
	return fmt.Sprintf("%v", ci.Value) == fmt.Sprintf("%v", ci.Default)
}

// MaskedValue 获取脱敏值（用于展示）
func (ci *ConfigItem) MaskedValue() string {
	if !ci.Sensitive {
		return ci.StringValue()
	}
	v := ci.StringValue()
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + "****" + v[len(v)-2:]
}

// ============================================================================
// 二、验证规则
// ============================================================================

// ValidationRule 验证规则
type ValidationRule struct {
	Required  bool     `json:"required"`              // 是否必填
	MinValue  *float64 `json:"min_value,omitempty"`   // 最小值（数值）
	MaxValue  *float64 `json:"max_value,omitempty"`   // 最大值（数值）
	MinLength *int     `json:"min_length,omitempty"`  // 最小长度（字符串）
	MaxLength *int     `json:"max_length,omitempty"`  // 最大长度（字符串）
	Pattern   string   `json:"pattern,omitempty"`     // 正则表达式
	Enum      []string `json:"enum,omitempty"`        // 枚举值
}

// Validate 验证配置值
func (vr *ValidationRule) Validate(value interface{}) error {
	if vr == nil {
		return nil
	}
	if value == nil {
		if vr.Required {
			return fmt.Errorf("value is required")
		}
		return nil
	}

	str := fmt.Sprintf("%v", value)

	// 枚举检查
	if len(vr.Enum) > 0 {
		found := false
		for _, e := range vr.Enum {
			if e == str {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value must be one of: %v", vr.Enum)
		}
	}

	// 长度检查
	if vr.MinLength != nil && len(str) < *vr.MinLength {
		return fmt.Errorf("value length must be at least %d", *vr.MinLength)
	}
	if vr.MaxLength != nil && len(str) > *vr.MaxLength {
		return fmt.Errorf("value length must be at most %d", *vr.MaxLength)
	}

	// 数值范围检查
	if vr.MinValue != nil || vr.MaxValue != nil {
		var f float64
		switch v := value.(type) {
		case int:
			f = float64(v)
		case int64:
			f = float64(v)
		case float64:
			f = v
		case float32:
			f = float64(v)
		default:
			return fmt.Errorf("value must be numeric")
		}
		if vr.MinValue != nil && f < *vr.MinValue {
			return fmt.Errorf("value must be at least %v", *vr.MinValue)
		}
		if vr.MaxValue != nil && f > *vr.MaxValue {
			return fmt.Errorf("value must be at most %v", *vr.MaxValue)
		}
	}

	return nil
}

// ============================================================================
// 三、配置快照
// ============================================================================

// ConfigSnapshot 配置快照
type ConfigSnapshot struct {
	Version     string                 `json:"version"`
	Items       map[string]*ConfigItem `json:"items"`
	Description string                 `json:"description"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              
	CreatedBy   string                 `json:"created_by"`
	Tags        []string               `json:"tags,omitempty"`
	// 变更来源
	Source string `json:"source"` // api/file/ui
}

// DeepCopy 深拷贝快照
func (cs *ConfigSnapshot) DeepCopy() *ConfigSnapshot {
	items := make(map[string]*ConfigItem, len(cs.Items))
	for k, v := range cs.Items {
		items[k] = &ConfigItem{
			Key:         v.Key,
			Value:       v.Value,
			Type:        v.Type,
			Description: v.Description,
			Category:    v.Category,
			Default:     v.Default,
			Editable:    v.Editable,
			Sensitive:   v.Sensitive,
			Validation:  v.Validation,
			Source:      v.Source,
			CreatedAt:   v.CreatedAt,
			UpdatedAt:   v.UpdatedAt,
			UpdatedBy:   v.UpdatedBy,
		}
	}
	return &ConfigSnapshot{
		Version:     cs.Version,
		Items:       items,
		Description: cs.Description,
		CreatedAt:   cs.CreatedAt,
		CreatedBy:   cs.CreatedBy,
		Tags:        append([]string{}, cs.Tags...),
		Source:      cs.Source,
	}
}

// GetItem 获取配置项
func (cs *ConfigSnapshot) GetItem(key string) *ConfigItem {
	if cs.Items == nil {
		return nil
	}
	return cs.Items[key]
}

// SetItem 设置配置项
func (cs *ConfigSnapshot) SetItem(item *ConfigItem) {
	if cs.Items == nil {
		cs.Items = make(map[string]*ConfigItem)
	}
	cs.Items[item.Key] = item
}

// Compare 比较两个快照的差异
func (cs *ConfigSnapshot) Compare(other *ConfigSnapshot) *ConfigDiff {
	if cs.Items == nil || other == nil || other.Items == nil {
		return nil
	}

	diff := &ConfigDiff{
		FromVersion: cs.Version,
		ToVersion:   other.Version,
		At:          time.Now(),
	}

	allKeys := make(map[string]bool)
	for k := range cs.Items {
		allKeys[k] = true
	}
	for k := range other.Items {
		allKeys[k] = true
	}

	for k := range allKeys {
		oldItem := cs.Items[k]
		newItem := other.Items[k]

		if oldItem == nil && newItem != nil {
			diff.Added = append(diff.Added, k)
		} else if oldItem != nil && newItem == nil {
			diff.Removed = append(diff.Removed, k)
		} else if oldItem != nil && newItem != nil {
			if fmt.Sprintf("%v", oldItem.Value) != fmt.Sprintf("%v", newItem.Value) {
				diff.Changed = append(diff.Changed, k)
			}
		}
	}

	return diff
}

// ToJSON 导出为 JSON
func (cs *ConfigSnapshot) ToJSON() (string, error) {
	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON 从 JSON 导入
func FromJSON(data []byte) (*ConfigSnapshot, error) {
	var cs ConfigSnapshot
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, err
	}
	return &cs, nil
}

// ============================================================================
// 四、配置差异
// ============================================================================

// ConfigDiff 配置差异
type ConfigDiff struct {
	FromVersion string    `json:"from_version"`
	ToVersion   string    `json:"to_version"`
	Added       []string  `json:"added"`
	Removed     []string  `json:"removed"`
	Changed     []string  `json:"changed"`
	At          time.Time `json:"at"`
}

// IsEmpty 是否无差异
func (cd *ConfigDiff) IsEmpty() bool {
	return len(cd.Added) == 0 && len(cd.Removed) == 0 && len(cd.Changed) == 0
}

// Summary 返回差异摘要
func (cd *ConfigDiff) Summary() string {
	return fmt.Sprintf("added: %d, removed: %d, changed: %d", len(cd.Added), len(cd.Removed), len(cd.Changed))
}
