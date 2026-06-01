//go:build linux

// Package profiler 提供 ON-CPU 性能剖析功能
// 本文件封装了 Linux perf_event 系统调用，用于 CPU 采样
package profiler

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

// ==================== perf_event 常量定义 ====================

// perf_event_attr.type 字段 - 事件类型
const (
	PERF_TYPE_HARDWARE  = 0  // 硬件事件
	PERF_TYPE_SOFTWARE  = 1  // 软件事件
	PERF_TYPE_TRACEPOINT = 2 // 跟踪点事件
	PERF_TYPE_HW_CACHE  = 3  // 硬件缓存事件
	PERF_TYPE_RAW       = 4  // 原始事件
	PERF_TYPE_BREAKPOINT = 5 // 断点事件
)

// PERF_TYPE_SOFTWARE 对应的 config 值 - 软件事件类型
const (
	PERF_COUNT_SW_CPU_CLOCK        = 0   // CPU 时钟事件
	PERF_COUNT_SW_TASK_CLOCK       = 1   // 任务时钟事件
	PERF_COUNT_SW_PAGE_FAULTS      = 2   // 缺页中断
	PERF_COUNT_SW_CONTEXT_SWITCHES = 3   // 上下文切换
	PERF_COUNT_SW_CPU_MIGRATIONS   = 4   // CPU 迁移
	PERF_COUNT_SW_PAGE_FAULTS_MIN  = 5   // 次要缺页中断
	PERF_COUNT_SW_PAGE_FAULTS_MAJ  = 6   // 主要缺页中断
	PERF_COUNT_SW_ALIGNMENT_FAULTS = 7   // 对齐错误
	PERF_COUNT_SW_EMULATION_FAULTS = 8   // 模拟错误
	PERF_COUNT_SW_DUMMY            = 9   // 虚拟事件
	PERF_COUNT_SW_BPF_OUTPUT       = 10  // BPF 输出事件
)

// PERF_TYPE_HARDWARE 对应的 config 值 - 硬件事件类型
const (
	PERF_COUNT_HW_CPU_CYCLES        = 0 // CPU 周期
	PERF_COUNT_HW_INSTRUCTIONS      = 1 // 指令数
	PERF_COUNT_HW_CACHE_REFERENCES  = 2 // 缓存引用
	PERF_COUNT_HW_CACHE_MISSES      = 3 // 缓存未命中
	PERF_COUNT_HW_BRANCH_INSTRUCTIONS = 4 // 分支指令
	PERF_COUNT_HW_BRANCH_MISSES     = 5 // 分支预测失败
	PERF_COUNT_HW_BUS_CYCLES        = 6 // 总线周期
	PERF_COUNT_HW_STALLED_CYCLES_FRONTEND = 7 // 前端停顿周期
	PERF_COUNT_HW_STALLED_CYCLES_BACKEND  = 8 // 后端停顿周期
	PERF_COUNT_HW_REF_CPU_CYCLES    = 9 // 引用 CPU 周期
)

// perf_event_attr.sample_type 字段 - 采样类型位掩码
const (
	PERF_SAMPLE_IP           = 1 << 0  // 指令指针
	PERF_SAMPLE_TID          = 1 << 1  // 线程/进程 ID
	PERF_SAMPLE_TIME         = 1 << 2  // 时间戳
	PERF_SAMPLE_ADDR         = 1 << 3  // 地址
	PERF_SAMPLE_READ         = 1 << 4  // 读取值
	PERF_SAMPLE_CALLCHAIN    = 1 << 5  // 调用链
	PERF_SAMPLE_ID           = 1 << 6  // 采样 ID
	PERF_SAMPLE_CPU          = 1 << 7  // CPU 编号
	PERF_SAMPLE_PERIOD       = 1 << 8  // 采样周期
	PERF_SAMPLE_STREAM_ID    = 1 << 9  // 流 ID
	PERF_SAMPLE_RAW          = 1 << 10 // 原始数据
	PERF_SAMPLE_BRANCH_STACK = 1 << 11 // 分支栈
	PERF_SAMPLE_REGS_USER    = 1 << 12 // 用户态寄存器
	PERF_SAMPLE_STACK_USER   = 1 << 13 // 用户态栈
	PERF_SAMPLE_WEIGHT       = 1 << 14 // 权重
	PERF_SAMPLE_DATA_SRC     = 1 << 15 // 数据源
	PERF_SAMPLE_IDENTIFIER   = 1 << 16 // 标识符
	PERF_SAMPLE_REGS_INTR    = 1 << 17 // 中断寄存器
	PERF_SAMPLE_PHYS_ADDR    = 1 << 18 // 物理地址
	PERF_SAMPLE_AUX          = 1 << 19 // 辅助数据
	PERF_SAMPLE_CGROUP       = 1 << 20 // 控制组
	PERF_SAMPLE_DATA_PAGE_SIZE = 1 << 21 // 数据页大小
	PERF_SAMPLE_CODE_PAGE_SIZE = 1 << 22 // 代码页大小
	PERF_SAMPLE_WEIGHT_STRUCT  = 1 << 23 // 权重结构
)

// perf_event_attr.read_format 字段 - 读取格式位掩码
const (
	PERF_FORMAT_TOTAL           = 1 << 0
	PERF_FORMAT_ID              = 1 << 1
	PERF_FORMAT_GROUP           = 1 << 2
	PERF_FORMAT_LOST            = 1 << 3
)

// perf_event_attr.flags 字段 - 属性标志位
const (
	PERF_FLAG_FD_NO_GROUP  = 1 << 0
	PERF_FLAG_FD_OUTPUT    = 1 << 1
	PERF_FLAG_PID_CGROUP   = 1 << 2
	PERF_FLAG_FD_CLOEXEC   = 1 << 3
)

// perf_event_attr 中的其他标志
const (
	PERF_ATTR_FLAG_DISABLED    = 1 << 0 // 初始禁用
	PERF_ATTR_FLAG_INHERIT     = 1 << 1 // 继承到子进程
	PERF_ATTR_FLAG_PINNED      = 1 << 2 // 固定到 CPU
	PERF_ATTR_FLAG_EXCLUSIVE   = 1 << 3 // 独占事件组
	PERF_ATTR_FLAG_EXCLUDE_USER   = 1 << 4  // 排除用户态
	PERF_ATTR_FLAG_EXCLUDE_KERNEL = 1 << 5  // 排除内核态
	PERF_ATTR_FLAG_EXCLUDE_HV     = 1 << 6  // 排除虚拟机管理器
	PERF_ATTR_FLAG_EXCLUDE_IDLE   = 1 << 7  // 排除空闲
	PERF_ATTR_FLAG_MMAP       = 1 << 8  // 映射事件
	PERF_ATTR_FLAG_COMM       = 1 << 9  // 通信事件
	PERF_ATTR_FLAG_FREQ       = 1 << 10 // 使用频率而非周期
	PERF_ATTR_FLAG_INHERIT_STAT = 1 << 11 // 继承统计
	PERF_ATTR_FLAG_ENABLE_ON_EXEC = 1 << 12 // exec 时启用
	PERF_ATTR_FLAG_TASK       = 1 << 13 // 跟踪任务
	PERF_ATTR_FLAG_WATERMARK  = 1 << 14 // 水位标记
	PERF_ATTR_FLAG_USE_CLOCKID = 1 << 15 // 使用指定时钟
	PERF_ATTR_FLAG_CONTEXT_SWITCH = 1 << 16 // 上下文切换
)

// ioctl 命令常量
const (
	PERF_EVENT_IOC_ENABLE   = 0x2400 // 启用事件
	PERF_EVENT_IOC_DISABLE  = 0x2401 // 禁用事件
	PERF_EVENT_IOC_RESET    = 0x2403 // 重置事件计数
	PERF_EVENT_IOC_REFRESH  = 0x2402 // 刷新事件
	PERF_EVENT_IOC_SET_PERIOD = 0x40082404 // 设置采样周期
	PERF_EVENT_IOC_SET_FILTER = 0x40082406 // 设置过滤条件
	PERF_EVENT_IOC_SET_BPF   = 0x40042408 // 设置 BPF 程序
	PERF_EVENT_IOC_PAUSE_OUTPUT = 0x40042409 // 暂停输出
	PERF_EVENT_IOC_QUERY_BPF = 0xC008240A // 查询 BPF 程序
)

// mmap ring buffer 相关常量
const (
	PERF_EVENT_MMAP_SIZE = 128 * 1024 // ring buffer 大小 (128KB)，必须是 2 的幂次 + 1 页
	PERF_MMAP_PAGE_SIZE  = 4096       // perf mmap 的元数据页大小
)

// Linux 系统调用号
const (
	SYS_PERF_EVENT_OPEN = 298 // x86_64 架构的 perf_event_open 系统调用号
	SYS_IOCTL           = 16  // ioctl 系统调用号
	SYS_MMAP            = 9   // mmap 系统调用号
	SYS_MUNMAP          = 11  // munmap 系统调用号
	SYS_CLOSE           = 3   // close 系统调用号
)

// ==================== perf_event_attr 结构体 ====================

// perf_event_attr 封装 Linux 内核的 perf_event_attr 结构体
// 用于配置 perf 采样事件的属性
// 参考: https://man7.org/linux/man-pages/man2/perf_event_open.2.html
type perfEventAttr struct {
	Type          uint32 // 事件类型 (PERF_TYPE_SOFTWARE 等)
	Size          uint32 // 结构体大小
	Config        uint64 // 事件配置 (PERF_COUNT_SW_CPU_CLOCK 等)
	SamplePeriod  uint64 // 采样周期 (与 freq 互斥)
	SampleFreq    uint64 // 采样频率 (与 period 互斥)
	SampleType    uint64 // 采样类型位掩码
	ReadFormat    uint64 // 读取格式位掩码
	Flags         uint64 // 属性标志位 (disabled, inherit, freq 等)
	BPType        uint32 // 断点类型 (仅用于 PERF_TYPE_BREAKPOINT)
	BPAddr        uint64 // 断点地址
	BPLen         uint32 // 断点长度
	BranchType    uint32 // 分支类型
	SampleRegsUser  uint64 // 采样时保存的用户态寄存器
	SampleStackUser uint32 // 采样时保存的用户态栈大小
	ClockID       int32  // 时钟 ID (当 use_clockid 标志设置时)
	SampleRegsIntr uint64 // 采样时保存的中断寄存器
	AuxWatermark  uint32 // 辅助缓冲区水位标记
	SampleMaxStack uint16 // 最大栈深度
	Reserved      uint16 // 保留字段
}

// ==================== Ring Buffer 结构体 ====================

// perfEventHeader 是 ring buffer 中每个事件的头部
// 所有 perf 事件都以这个头部开始
type perfEventHeader struct {
	Type uint32 // 事件类型 (PERF_RECORD_SAMPLE 等)
	Misc uint16 // 杂项标志
	Size uint16 // 事件总大小 (包括头部)
}

// perfEventMmapEntry 是 PERF_RECORD_MMAP 事件的数据结构
// 包含内存映射信息，用于将地址映射到文件
type perfEventMmapEntry struct {
	Header perfEventHeader
	PID    uint32 // 进程 ID
	TID    uint32 // 线程 ID
	Addr   uint64 // 映射起始地址
	Len    uint64 // 映射长度
	Pgoff  uint64 // 文件偏移
	Filename [0]byte // 以 null 结尾的文件名 (变长)
}

// perfRingBuffer 封装 perf_event 的 mmap ring buffer
// 用于高效读取采样数据，避免频繁的系统调用
type perfRingBuffer struct {
	data     []byte       // mmap 映射的内存区域
	dataSize int          // 数据区域大小 (总大小 - 元数据页)
	fd       int          // perf_event 文件描述符
	header   *perfMmapPage // 指向元数据页的指针
}

// perfMmapPage 是 ring buffer 的元数据页结构
// 位于 mmap 区域的第一页，包含 ring buffer 的控制信息
type perfMmapPage struct {
	Version        uint32        // 版本号
	CompatVersion  uint32        // 兼容版本
	Lock           uint32        // 自旋锁
	Index          uint32        // ring buffer 索引
	Offset         int64         // 数据偏移
	TimeEnabled    uint64        // 事件启用时间
	TimeRunning    uint64        // 事件运行时间
	Capabilities   uint64        // 能力标志
	PadHead        uint64        // 头部填充
	PadTail        uint64        // 尾部填充
	PadT0Width     uint64        // 时间戳宽度
	PadT1Width     uint64        // 时间戳宽度
	AuxHead        uint64        // 辅助缓冲区头部
	AuxTail        uint64        // 辅助缓冲区尾部
	AuxOffset      uint64        // 辅助缓冲区偏移
}

// perf 事件记录类型常量
const (
	PERF_RECORD_MMAP          = 1  // 内存映射事件
	PERF_RECORD_LOST          = 2  // 丢失事件
	PERF_RECORD_COMM          = 3  // 进程名变更事件
	PERF_RECORD_EXIT          = 4  // 进程退出事件
	PERF_RECORD_THROTTLE      = 5  // 节流事件
	PERF_RECORD_UNTHROTTLE    = 6  // 取消节流事件
	PERF_RECORD_FORK          = 7  // 进程 fork 事件
	PERF_RECORD_READ          = 8  // 读取事件
	PERF_RECORD_SAMPLE        = 9  // 采样事件 (核心)
	PERF_RECORD_MMAP2         = 10 // 扩展内存映射事件
	PERF_RECORD_AUX           = 11 // 辅助数据事件
	PERF_RECORD_ITRACE_START  = 12 // 指令跟踪开始
	PERF_RECORD_LOST_SAMPLES  = 13 // 丢失采样事件
	PERF_RECORD_SWITCH        = 14 // 上下文切换事件
	PERF_RECORD_SWITCH_CPU_WIDE = 15 // CPU 宽上下文切换
	PERF_RECORD_NAMESPACES    = 16 // 命名空间事件
	PERF_RECORD_KSYMBOL       = 17 // 内核符号事件
	PERF_RECORD_BPF_EVENT     = 18 // BPF 事件
	PERF_RECORD_CGROUP        = 19 // 控制组事件
	PERF_RECORD_TEXT_POKE     = 20 // 文本修改事件
	PERF_RECORD_AUX_OUTPUT_HW = 21 // 辅助输出硬件事件
)

// ==================== perf_event 系统调用封装 ====================

// OpenPerfEvent 打开一个 perf 事件用于 CPU 采样
// 参数:
//   - cpu: 要监控的 CPU 编号，-1 表示所有 CPU
//   - freq: 采样频率 (每秒采样次数)，0 使用默认值 99
//   - pid: 目标进程 ID，-1 表示所有进程，0 表示当前进程
//
// 返回:
//   - fd: perf_event 文件描述符
//   - error: 错误信息
func OpenPerfEvent(cpu int, freq uint64, pid int) (int, error) {
	// 如果频率为 0，使用默认值 99Hz
	if freq == 0 {
		freq = 99
	}

	// 构造 perf_event_attr 结构体
	// 使用软件事件 CPU_CLOCK 进行采样，兼容性最好
	attr := &perfEventAttr{
		Type:       PERF_TYPE_SOFTWARE, // 使用软件事件
		Config:     PERF_COUNT_SW_CPU_CLOCK, // CPU 时钟事件
		SampleFreq: freq,               // 采样频率
		SampleType: PERF_SAMPLE_CPU | PERF_SAMPLE_STACK_USER | PERF_SAMPLE_STACK_KERNEL |
			PERF_SAMPLE_TIME | PERF_SAMPLE_TID | PERF_SAMPLE_PERIOD | PERF_SAMPLE_CALLCHAIN,
		ReadFormat: PERF_FORMAT_TOTAL | PERF_FORMAT_LOST | PERF_FORMAT_ID,
		Flags: PERF_ATTR_FLAG_DISABLED | // 初始禁用，手动启用
			PERF_ATTR_FLAG_FREQ | // 使用频率模式
			PERF_ATTR_FLAG_MMAP | // 启用 mmap 映射
			PERF_ATTR_FLAG_COMM | // 跟踪进程名变更
			PERF_ATTR_FLAG_TASK | // 跟踪任务
			PERF_ATTR_FLAG_EXCLUDE_IDLE, // 排除空闲时间
		SampleStackUser: 8192, // 用户态栈最大采样大小 (8KB)
		SampleMaxStack:  127,  // 最大调用链深度
	}
	attr.Size = uint32(unsafe.Sizeof(*attr))

	// 调用 perf_event_open 系统调用
	// 系统调用号: 298 (x86_64)
	// 参数: attr 指针, pid, cpu, group_fd, flags
	fd, _, errno := syscall.Syscall6(
		SYS_PERF_EVENT_OPEN,
		uintptr(unsafe.Pointer(attr)),
		uintptr(pid),
		uintptr(cpu),
		0, // group_fd = -1 (无组)
		uintptr(PERF_FLAG_FD_CLOEXEC), // 设置 close-on-exec
		0,
	)

	if int(errno) != 0 {
		return -1, fmt.Errorf("perf_event_open 失败: errno=%d, 错误=%s", errno, errno)
	}

	return int(fd), nil
}

// ClosePerfEvent 关闭 perf 事件文件描述符
// 释放系统资源
func ClosePerfEvent(fd int) error {
	if fd < 0 {
		return fmt.Errorf("无效的文件描述符: %d", fd)
	}
	_, _, errno := syscall.Syscall(SYS_CLOSE, uintptr(fd), 0, 0)
	if int(errno) != 0 {
		return fmt.Errorf("关闭 perf event 失败: errno=%d", errno)
	}
	return nil
}

// EnablePerfEvent 启用 perf 事件采样
// 通过 ioctl 系统调用发送 PERF_EVENT_IOC_ENABLE 命令
func EnablePerfEvent(fd int) error {
	if fd < 0 {
		return fmt.Errorf("无效的文件描述符: %d", fd)
	}
	_, _, errno := syscall.Syscall(
		SYS_IOCTL,
		uintptr(fd),
		uintptr(PERF_EVENT_IOC_ENABLE),
		0,
	)
	if int(errno) != 0 {
		return fmt.Errorf("启用 perf event 失败: errno=%d", errno)
	}
	return nil
}

// DisablePerfEvent 禁用 perf 事件采样
// 通过 ioctl 系统调用发送 PERF_EVENT_IOC_DISABLE 命令
func DisablePerfEvent(fd int) error {
	if fd < 0 {
		return fmt.Errorf("无效的文件描述符: %d", fd)
	}
	_, _, errno := syscall.Syscall(
		SYS_IOCTL,
		uintptr(fd),
		uintptr(PERF_EVENT_IOC_DISABLE),
		0,
	)
	if int(errno) != 0 {
		return fmt.Errorf("禁用 perf event 失败: errno=%d", errno)
	}
	return nil
}

// SetPeriod 动态设置 perf 事件的采样周期
// 通过 ioctl 系统调用发送 PERF_EVENT_IOC_SET_PERIOD 命令
// 注意: 设置周期后，如果使用了 freq 模式，内核会自动调整
func SetPeriod(fd int, period uint64) error {
	if fd < 0 {
		return fmt.Errorf("无效的文件描述符: %d", fd)
	}
	// 将 period 值写入内核
	_, _, errno := syscall.Syscall(
		SYS_IOCTL,
		uintptr(fd),
		uintptr(PERF_EVENT_IOC_SET_PERIOD),
		uintptr(period),
	)
	if int(errno) != 0 {
		return fmt.Errorf("设置采样周期失败: errno=%d", errno)
	}
	return nil
}

// mmapRingBuffer 将 perf 事件文件描述符映射到内存
// 创建 ring buffer 用于高效读取采样数据
func mmapRingBuffer(fd int, size int) (*perfRingBuffer, error) {
	if fd < 0 {
		return nil, fmt.Errorf("无效的文件描述符: %d", fd)
	}

	// mmap 映射 perf_event ring buffer
	// 参数: addr=NULL, length=1页+数据区, prot=READ|WRITE, flags=SHARED, fd, offset=0
	data, _, errno := syscall.Syscall6(
		SYS_MMAP,
		0,                                  // addr = NULL，由内核选择地址
		uintptr(size),                      // length = 元数据页 + 数据区
		syscall.PROT_READ|syscall.PROT_WRITE, // prot = 可读可写
		syscall.MAP_SHARED,                 // flags = 共享映射
		uintptr(fd),                        // fd = perf_event 文件描述符
		0,                                  // offset = 0
	)

	if int(errno) != 0 {
		return nil, fmt.Errorf("mmap ring buffer 失败: errno=%d", errno)
	}

	if data == ^uintptr(0) {
		return nil, fmt.Errorf("mmap 返回 MAP_FAILED")
	}

	// 将 uintptr 转换为 []byte 切片
	// 注意: 这里使用 unsafe 将映射的内存转换为字节切片
	header := (*perfMmapPage)(unsafe.Pointer(data))

	buf := &perfRingBuffer{
		fd:       fd,
		dataSize: size - PERF_MMAP_PAGE_SIZE,
		header:   header,
	}

	// 构造数据区域的切片
	sliceHeader := (*struct {
		data uintptr
		len  int
		cap  int
	})(unsafe.Pointer(&buf.data))
	sliceHeader.data = data + PERF_MMAP_PAGE_SIZE
	sliceHeader.len = buf.dataSize
	sliceHeader.cap = buf.dataSize

	return buf, nil
}

// munmapRingBuffer 解除 ring buffer 的内存映射
func munmapRingBuffer(buf *perfRingBuffer) error {
	if buf == nil || len(buf.data) == 0 {
		return nil
	}

	// 计算总映射大小 (元数据页 + 数据区)
	totalSize := PERF_MMAP_PAGE_SIZE + buf.dataSize
	_, _, errno := syscall.Syscall(
		SYS_MUNMAP,
		uintptr(unsafe.Pointer(&buf.data[0]))-PERF_MMAP_PAGE_SIZE,
		uintptr(totalSize),
		0,
	)
	if int(errno) != 0 {
		return fmt.Errorf("munmap ring buffer 失败: errno=%d", errno)
	}
	buf.data = nil
	return nil
}

// ReadRingBuffer 从 perf ring buffer 中读取采样数据
// handler 回调函数接收每个采样的原始字节数据
// 该函数会阻塞直到有数据可读或发生错误
func ReadRingBuffer(fd int, handler func(raw []byte)) error {
	if fd < 0 {
		return fmt.Errorf("无效的文件描述符: %d", fd)
	}

	// 映射 ring buffer
	bufSize := PERF_MMAP_PAGE_SIZE + PERF_EVENT_MMAP_SIZE
	buf, err := mmapRingBuffer(fd, bufSize)
	if err != nil {
		return fmt.Errorf("映射 ring buffer 失败: %w", err)
	}
	defer munmapRingBuffer(buf)

	// 持续读取 ring buffer 中的事件
	for {
		// 获取数据头部和尾部的位置
		dataHead := atomicLoadUint64(&buf.header.Offset)
		dataTail := buf.header.Offset // 尾部位置

		// 读取 ring buffer 中的所有可用数据
		// ring buffer 是循环缓冲区，需要处理回绕情况
		for dataTail < dataHead {
			// 计算当前事件在数据区中的偏移
			offset := dataTail % uint64(buf.dataSize)

			// 读取事件头部
			if offset+uint64(unsafe.Sizeof(perfEventHeader{})) > uint64(buf.dataSize) {
				// 事件跨页，跳过 (简化处理)
				break
			}

			// 解析事件头部
			eventHeader := (*perfEventHeader)(unsafe.Pointer(&buf.data[offset]))
			eventSize := uint64(eventHeader.Size)

			// 检查事件大小是否有效
			if eventSize < uint64(unsafe.Sizeof(perfEventHeader{})) || eventSize > uint64(buf.dataSize) {
				break
			}

			// 只处理采样事件 (PERF_RECORD_SAMPLE)
			if eventHeader.Type == PERF_RECORD_SAMPLE {
				// 提取事件数据 (去掉头部)
				eventData := buf.data[offset : offset+eventSize]
				handler(eventData)
			}

			// 移动到下一个事件
			dataTail += eventSize
		}

		// 更新尾部位置，通知内核已消费数据
		atomicStoreUint64(&buf.header.Offset, dataTail)
	}
}

// ==================== 辅助函数 ====================

// atomicLoadUint64 原子加载 uint64 值
// 用于安全读取 ring buffer 的头部指针
func atomicLoadUint64(addr *uint64) uint64 {
	// 使用 unsafe 直接读取，在 x86_64 上 uint64 读取是原子的
	return *(*uint64)(unsafe.Pointer(addr))
}

// atomicStoreUint64 原子存储 uint64 值
// 用于安全更新 ring buffer 的尾部指针
func atomicStoreUint64(addr *uint64, val uint64) {
	*(*uint64)(unsafe.Pointer(addr)) = val
}

// parseSampleEvent 解析 PERF_RECORD_SAMPLE 事件
// 从原始字节数据中提取采样信息
// 返回: CPU 编号、时间戳、PID/TID、调用链、用户栈数据
func parseSampleEvent(data []byte) (cpu uint32, timestamp uint64, pid uint32, tid uint32, callchain []uint64, userStack []byte) {
	if len(data) < int(unsafe.Sizeof(perfEventHeader{})) {
		return
	}

	// 跳过事件头部
	offset := uint64(unsafe.Sizeof(perfEventHeader{}))

	// 解析 PERF_SAMPLE_TID (PID + TID)
	pid = binary.LittleEndian.Uint32(data[offset:])
	tid = binary.LittleEndian.Uint32(data[offset+4:])
	offset += 8

	// 解析 PERF_SAMPLE_TIME
	timestamp = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// 解析 PERF_SAMPLE_CPU
	cpu = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	_ = binary.LittleEndian.Uint32(data[offset:]) // res
	offset += 4

	// 解析 PERF_SAMPLE_PERIOD
	_ = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// 解析 PERF_SAMPLE_STACK_USER
	stackSize := binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	dynSize := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	if dynSize > 0 && stackSize > 0 {
		userStack = data[offset : offset+dynSize]
		offset += dynSize
		// 对齐到 8 字节
		if offset%8 != 0 {
			offset += 8 - offset%8
		}
	}

	// 解析 PERF_SAMPLE_STACK_KERNEL
	// 内核栈数据 (如果有)
	// 注意: 实际解析需要根据 sample_type 中是否包含 PERF_SAMPLE_STACK_KERNEL

	// 解析 PERF_SAMPLE_CALLCHAIN
	if offset < uint64(len(data))-8 {
		nr := binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		callchain = make([]uint64, nr)
		for i := uint64(0); i < nr && offset+8 <= uint64(len(data)); i++ {
			callchain[i] = binary.LittleEndian.Uint64(data[offset:])
			offset += 8
		}
	}

	return
}

// NewPerfRingBuffer 创建新的 perf ring buffer
// 这是面向外部的便捷接口
func NewPerfRingBuffer(fd int) (*perfRingBuffer, error) {
	bufSize := PERF_MMAP_PAGE_SIZE + PERF_EVENT_MMAP_SIZE
	return mmapRingBuffer(fd, bufSize)
}

// Close 关闭 ring buffer 并释放资源
func (rb *perfRingBuffer) Close() error {
	return munmapRingBuffer(rb)
}

// ReadAvailable 读取 ring buffer 中所有可用的采样数据
// 非阻塞方式，读取当前所有可用数据后返回
func (rb *perfRingBuffer) ReadAvailable(handler func(raw []byte)) error {
	if rb == nil || rb.header == nil {
		return fmt.Errorf("ring buffer 未初始化")
	}

	// 获取数据头部位置
	dataHead := atomicLoadUint64(&rb.header.Offset)
	dataTail := rb.header.Offset

	// 遍历所有可用事件
	for dataTail < dataHead {
		offset := dataTail % uint64(rb.dataSize)

		// 检查是否有足够的数据读取事件头部
		if offset+uint64(unsafe.Sizeof(perfEventHeader{})) > uint64(rb.dataSize) {
			break
		}

		// 解析事件头部
		eventHeader := (*perfEventHeader)(unsafe.Pointer(&rb.data[offset]))
		eventSize := uint64(eventHeader.Size)

		// 验证事件大小
		if eventSize < uint64(unsafe.Sizeof(perfEventHeader{})) || eventSize > uint64(rb.dataSize) {
			break
		}

		// 处理采样事件
		if eventHeader.Type == PERF_RECORD_SAMPLE {
			eventData := rb.data[offset : offset+eventSize]
			handler(eventData)
		}

		// 前进到下一个事件
		dataTail += eventSize
	}

	// 更新尾部位置
	atomicStoreUint64(&rb.header.Offset, dataTail)

	return nil
}
