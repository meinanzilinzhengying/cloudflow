// Package adapter Casbin Gorm Adapter
//
// P1-03 修复: RBAC 持久化
//
// 提供 Casbin 持久化适配器，将策略和角色映射存储到 TiDB/MySQL。
package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"gorm.io/gorm"
)

// GormAdapter Gorm 持久化适配器
type GormAdapter struct {
	db                *gorm.DB
	isFiltered        bool
	autoFlush        bool
	tableName         string
}

// GormAdapterOption GormAdapter 配置选项
type GormAdapterOption func(*GormAdapter)

// WithTableName 设置表名
func WithTableName(name string) GormAdapterOption {
	return func(a *GormAdapter) {
		a.tableName = name
	}
}

// WithAutoFlush 设置是否自动刷新到数据库
func WithAutoFlush(autoFlush bool) GormAdapterOption {
	return func(a *GormAdapter) {
		a.autoFlush = autoFlush
	}
}

// NewGormAdapter 创建 Gorm Adapter
//
// P1-03 修复: RBAC 持久化到 TiDB
func NewGormAdapter(db *gorm.DB, opts ...GormAdapterOption) (*GormAdapter, error) {
	a := &GormAdapter{
		db:         db,
		isFiltered: false,
		autoFlush: true,
		tableName:  "casbin_rules",
	}

	for _, opt := range opts {
		opt(a)
	}

	// 自动迁移表结构
	if err := a.autoMigrate(); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return a, nil
}

// autoMigrate 自动迁移表结构
func (a *GormAdapter) autoMigrate() error {
	type CasbinRule struct {
		ID    uint   `gorm:"primaryKey;autoIncrement"`
		PType string `gorm:"size:100;index;uniqueIndex:idx_p_type_v0_v1_v2"`
		V0    string `gorm:"size:100;index"`
		V1    string `gorm:"size:100;index"`
		V2    string `gorm:"size:100;index"`
		V3    string `gorm:"size:100;index"`
		V4    string `gorm:"size:100;index"`
		V5    string `gorm:"size:100;index"`
	}

	if a.tableName != "" {
		if err := a.db.Table(a.tableName).AutoMigrate(&CasbinRule{}); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
	} else {
		if err := a.db.AutoMigrate(&CasbinRule{}); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
	}

	return nil
}

// LoadPolicy 从数据库加载所有策略到 Casbin model
func (a *GormAdapter) LoadPolicy(model model.Model) error {
	type CasbinRule struct {
		ID    uint
		PType string
		V0    string
		V1    string
		V2    string
		V3    string
		V4    string
		V5    string
	}

	var rules []CasbinRule
	var err error

	if a.tableName != "" {
		err = a.db.Table(a.tableName).Find(&rules).Error
	} else {
		err = a.db.Find(&rules).Error
	}

	if err != nil {
		return fmt.Errorf("load policy from db: %w", err)
	}

	for _, rule := range rules {
		loadPolicyRule(model, rule.PType, rule.V0, rule.V1, rule.V2, rule.V3, rule.V4, rule.V5)
	}

	return nil
}

// SavePolicy 将 Casbin model 中的策略保存到数据库
func (a *GormAdapter) SavePolicy(model model.Model) error {
	type CasbinRule struct {
		ID    uint   `gorm:"primaryKey"`
		PType string `gorm:"size:100;uniqueIndex:idx_p_type_v0_v1_v2"`
		V0    string `gorm:"size:100;index"`
		V1    string `gorm:"size:100;index"`
		V2    string `gorm:"size:100;index"`
		V3    string `gorm:"size:100;index"`
		V4    string `gorm:"size:100;index"`
		V5    string `gorm:"size:100;index"`
	}

	if a.tableName != "" {
		if err := a.db.Table(a.tableName).Delete(&CasbinRule{}).Error; err != nil {
			return fmt.Errorf("clear existing policies: %w", err)
		}
	} else {
		if err := a.db.Delete(&CasbinRule{}).Error; err != nil {
			return fmt.Errorf("clear existing policies: %w", err)
		}
	}

	// 保存 p 策略
	for _, ptype := range model["p"] {
		for _, rule := range ptype.Policy {
			cr := CasbinRule{
				PType: "p",
				V0:    getRuleValue(rule, 0),
				V1:    getRuleValue(rule, 1),
				V2:    getRuleValue(rule, 2),
				V3:    getRuleValue(rule, 3),
				V4:    getRuleValue(rule, 4),
				V5:    getRuleValue(rule, 5),
			}
			if a.tableName != "" {
				if err := a.db.Table(a.tableName).Create(&cr).Error; err != nil {
					return fmt.Errorf("save policy rule: %w", err)
				}
			} else {
				if err := a.db.Create(&cr).Error; err != nil {
					return fmt.Errorf("save policy rule: %w", err)
				}
			}
		}
	}

	// 保存 g 策略
	for _, ptype := range model["g"] {
		for _, rule := range ptype.Policy {
			cr := CasbinRule{
				PType: "g",
				V0:    getRuleValue(rule, 0),
				V1:    getRuleValue(rule, 1),
				V2:    getRuleValue(rule, 2),
				V3:    getRuleValue(rule, 3),
				V4:    getRuleValue(rule, 4),
				V5:    getRuleValue(rule, 5),
			}
			if a.tableName != "" {
				if err := a.db.Table(a.tableName).Create(&cr).Error; err != nil {
					return fmt.Errorf("save grouping policy rule: %w", err)
				}
			} else {
				if err := a.db.Create(&cr).Error; err != nil {
					return fmt.Errorf("save grouping policy rule: %w", err)
				}
			}
		}
	}

	return nil
}

// AddPolicy 添加单条策略
func (a *GormAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	if !a.isFiltered {
		return a.addPolicy(sec, ptype, rule)
	}
	return nil
}

// addPolicy 实际添加策略到数据库
func (a *GormAdapter) addPolicy(sec string, ptype string, rule []string) error {
	type CasbinRule struct {
		PType string `gorm:"size:100;uniqueIndex:idx_p_type_v0_v1_v2"`
		V0    string `gorm:"size:100;index"`
		V1    string `gorm:"size:100;index"`
		V2    string `gorm:"size:100;index"`
		V3    string `gorm:"size:100;index"`
		V4    string `gorm:"size:100;index"`
		V5    string `gorm:"size:100;index"`
	}

	cr := CasbinRule{
		PType: ptype,
		V0:    getRuleValue(rule, 0),
		V1:    getRuleValue(rule, 1),
		V2:    getRuleValue(rule, 2),
		V3:    getRuleValue(rule, 3),
		V4:    getRuleValue(rule, 4),
		V5:    getRuleValue(rule, 5),
	}

	var err error
	if a.tableName != "" {
		err = a.db.Table(a.tableName).Create(&cr).Error
	} else {
		err = a.db.Create(&cr).Error
	}

	if err != nil {
		return fmt.Errorf("add policy: %w", err)
	}

	return nil
}

// RemovePolicy 移除单条策略
func (a *GormAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	if !a.isFiltered {
		return a.removePolicy(sec, ptype, rule)
	}
	return nil
}

// removePolicy 实际从数据库移除策略
func (a *GormAdapter) removePolicy(sec string, ptype string, rule []string) error {
	type CasbinRule struct {
		PType string `gorm:"size:100"`
		V0    string `gorm:"size:100"`
		V1    string `gorm:"size:100"`
		V2    string `gorm:"size:100"`
		V3    string `gorm:"size:100"`
		V4    string `gorm:"size:100"`
		V5    string `gorm:"size:100"`
	}

	cr := CasbinRule{
		PType: ptype,
		V0:    getRuleValue(rule, 0),
		V1:    getRuleValue(rule, 1),
		V2:    getRuleValue(rule, 2),
		V3:    getRuleValue(rule, 3),
		V4:    getRuleValue(rule, 4),
		V5:    getRuleValue(rule, 5),
	}

	var err error
	if a.tableName != "" {
		err = a.db.Table(a.tableName).Where(&cr).Delete(&CasbinRule{}).Error
	} else {
		err = a.db.Where(&cr).Delete(&CasbinRule{}).Error
	}

	if err != nil {
		return fmt.Errorf("remove policy: %w", err)
	}

	return nil
}

// RemoveFilteredPolicy 移除匹配的策略
func (a *GormAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	if !a.isFiltered {
		return a.removeFilteredPolicy(sec, ptype, fieldIndex, fieldValues...)
	}
	return nil
}

// removeFilteredPolicy 实际从数据库移除匹配的策略
func (a *GormAdapter) removeFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	type CasbinRule struct {
		PType string `gorm:"size:100"`
		V0    string `gorm:"size:100"`
		V1    string `gorm:"size:100"`
		V2    string `gorm:"size:100"`
		V3    string `gorm:"size:100"`
		V4    string `gorm:"size:100"`
		V5    string `gorm:"size:100"`
	}

	query := &CasbinRule{PType: ptype}
	switch fieldIndex {
	case 0:
		query.V0 = fieldValues[0]
	case 1:
		query.V1 = fieldValues[0]
	case 2:
		query.V2 = fieldValues[0]
	case 3:
		query.V3 = fieldValues[0]
	case 4:
		query.V4 = fieldValues[0]
	case 5:
		query.V5 = fieldValues[0]
	}

	var err error
	if a.tableName != "" {
		err = a.db.Table(a.tableName).Where(query).Delete(&CasbinRule{}).Error
	} else {
		err = a.db.Where(query).Delete(&CasbinRule{}).Error
	}

	if err != nil {
		return fmt.Errorf("remove filtered policy: %w", err)
	}

	return nil
}

// AddPolicies 批量添加策略
func (a *GormAdapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	if !a.isFiltered {
		return a.addPolicies(sec, ptype, rules)
	}
	return nil
}

// addPolicies 实际批量添加策略到数据库
func (a *GormAdapter) addPolicies(sec string, ptype string, rules [][]string) error {
	type CasbinRule struct {
		PType string `gorm:"size:100;uniqueIndex:idx_p_type_v0_v1_v2"`
		V0    string `gorm:"size:100;index"`
		V1    string `gorm:"size:100;index"`
		V2    string `gorm:"size:100;index"`
		V3    string `gorm:"size:100;index"`
		V4    string `gorm:"size:100;index"`
		V5    string `gorm:"size:100;index"`
	}

	casbinRules := make([]CasbinRule, 0, len(rules))
	for _, rule := range rules {
		cr := CasbinRule{
			PType: ptype,
			V0:    getRuleValue(rule, 0),
			V1:    getRuleValue(rule, 1),
			V2:    getRuleValue(rule, 2),
			V3:    getRuleValue(rule, 3),
			V4:    getRuleValue(rule, 4),
			V5:    getRuleValue(rule, 5),
		}
		casbinRules = append(casbinRules, cr)
	}

	var err error
	if a.tableName != "" {
		err = a.db.Table(a.tableName).CreateInBatches(&casbinRules, 100).Error
	} else {
		err = a.db.CreateInBatches(&casbinRules, 100).Error
	}

	if err != nil {
		return fmt.Errorf("add policies in batch: %w", err)
	}

	return nil
}

// RemovePolicies 批量移除策略
func (a *GormAdapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	if !a.isFiltered {
		return a.removePolicies(sec, ptype, rules)
	}
	return nil
}

// removePolicies 实际批量从数据库移除策略
func (a *GormAdapter) removePolicies(sec string, ptype string, rules [][]string) error {
	type CasbinRule struct {
		PType string `gorm:"size:100"`
		V0    string `gorm:"size:100"`
		V1    string `gorm:"size:100"`
		V2    string `gorm:"size:100"`
		V3    string `gorm:"size:100"`
		V4    string `gorm:"size:100"`
		V5    string `gorm:"size:100"`
	}

	for _, rule := range rules {
		cr := CasbinRule{
			PType: ptype,
			V0:    getRuleValue(rule, 0),
			V1:    getRuleValue(rule, 1),
			V2:    getRuleValue(rule, 2),
			V3:    getRuleValue(rule, 3),
			V4:    getRuleValue(rule, 4),
			V5:    getRuleValue(rule, 5),
		}

		var err error
		if a.tableName != "" {
			err = a.db.Table(a.tableName).Where(&cr).Delete(&CasbinRule{}).Error
		} else {
			err = a.db.Where(&cr).Delete(&CasbinRule{}).Error
		}

		if err != nil {
			return fmt.Errorf("remove policy: %w", err)
		}
	}

	return nil
}

// AddGroupingPolicy 添加角色映射
func (a *GormAdapter) AddGroupingPolicy(rule []string) error {
	return a.AddPolicy("g", "g", rule)
}

// RemoveGroupingPolicy 移除角色映射
func (a *GormAdapter) RemoveGroupingPolicy(rule []string) error {
	return a.RemovePolicy("g", "g", rule)
}

// RemoveFilteredGroupingPolicy 移除匹配的角色映射
func (a *GormAdapter) RemoveFilteredGroupingPolicy(fieldIndex int, fieldValues ...string) error {
	return a.RemoveFilteredPolicy("g", "g", fieldIndex, fieldValues...)
}

// IsFiltered 是否使用过滤加载
func (a *GormAdapter) IsFiltered() bool {
	return a.isFiltered
}

// IsTableEmpty 检查表是否为空
func (a *GormAdapter) IsTableEmpty(ctx context.Context) (bool, error) {
	type CasbinRule struct {
		ID uint
	}

	var count int64
	var err error
	if a.tableName != "" {
		err = a.db.Table(a.tableName).WithContext(ctx).Model(&CasbinRule{}).Count(&count).Error
	} else {
		err = a.db.WithContext(ctx).Model(&CasbinRule{}).Count(&count).Error
	}

	if err != nil {
		return false, fmt.Errorf("count policies: %w", err)
	}

	return count == 0, nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// getRuleValue 安全获取规则值
func getRuleValue(rule []string, index int) string {
	if index < 0 || index >= len(rule) {
		return ""
	}
	return rule[index]
}

// loadPolicyRule 加载策略规则到 model
func loadPolicyRule(m model.Model, ptype string, v0 string, v1 string, v2 string, v3 string, v4 string, v5 string) {
	if strings.HasPrefix(ptype, "p") {
		if _, ok := m["p"]; !ok {
			m["p"] = model.NewPolicy()
		}
		rule := []string{v0, v1, v2, v3, v4}
		rule = filterEmptyStrings(rule)
		if len(rule) > 0 {
			m["p"].AddPolicy(rule)
		}
	} else if strings.HasPrefix(ptype, "g") {
		if _, ok := m["g"]; !ok {
			m["g"] = model.NewPolicy()
		}
		rule := []string{v0, v1, v2}
		rule = filterEmptyStrings(rule)
		if len(rule) > 0 {
			m["g"].AddPolicy(rule)
		}
	}
}

// filterEmptyStrings 过滤空字符串
func filterEmptyStrings(s []string) []string {
	result := make([]string, 0, len(s))
	for _, str := range s {
		if str != "" {
			result = append(result, str)
		}
	}
	return result
}

// Compile-time interface compliance check.
var _ persist.Adapter = (*GormAdapter)(nil)
