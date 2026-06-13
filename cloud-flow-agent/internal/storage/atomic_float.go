// Package storage 提供原子操作类型兼容层
package storage

import (
	"math"
	"sync/atomic"
)

// AtomicFloat64 原子浮点数（兼容 Go < 1.20 或自定义实现）
// 使用 atomic.Uint64 存储 float64 的位表示
type AtomicFloat64 struct {
	bits atomic.Uint64
}

// NewAtomicFloat64 创建新的原子浮点数
func NewAtomicFloat64(val float64) *AtomicFloat64 {
	a := &AtomicFloat64{}
	a.Store(val)
	return a
}

// Load 读取浮点数值
func (a *AtomicFloat64) Load() float64 {
	return math.Float64frombits(a.bits.Load())
}

// Store 存储浮点数值
func (a *AtomicFloat64) Store(val float64) {
	a.bits.Store(math.Float64bits(val))
}

// Add 原子加法（近似值，适用于指标采集）
func (a *AtomicFloat64) Add(delta float64) float64 {
	for {
		oldBits := a.bits.Load()
		oldVal := math.Float64frombits(oldBits)
		newVal := oldVal + delta
		newBits := math.Float64bits(newVal)
		if a.bits.CompareAndSwap(oldBits, newBits) {
			return newVal
		}
	}
}
