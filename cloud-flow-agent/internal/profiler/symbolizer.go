// Package profiler 提供 ON-CPU 性能剖析功能
// 本文件实现多语言符号解析器，支持 C/C++、Golang、Java 程序的地址到符号解析
package profiler

import (
	"bufio"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ==================== 符号解析相关结构体 ====================

// Symbol 表示一个解析后的符号信息
// 包含函数名、所在文件和行号
type Symbol struct {
	Name string // 函数名/符号名
	File string // 源代码文件路径
	Line int    // 源代码行号
}

// MemoryMapping 表示进程的一段内存映射
// 从 /proc/pid/maps 中解析得到
type MemoryMapping struct {
	StartAddr    uint64 // 映射起始虚拟地址
	EndAddr      uint64 // 映射结束虚拟地址
	Permissions  string // 权限 (r-xp 等)
	Offset       uint64 // 文件偏移量
	Device       string // 设备号 (如 08:01)
	Inode        uint64 // 文件 inode 号
	Pathname     string // 映射的文件路径 (可能为空，表示匿名映射)
}

// Symbolizer 多语言符号解析器
// 支持解析 C/C++、Golang、Java 程序的地址到函数名/文件名/行号
// 使用缓存机制避免重复解析 ELF 文件
type Symbolizer struct {
	mu           sync.RWMutex            // 读写锁，保护缓存
	elfCache     map[string]*elfFile     // ELF 文件缓存，key=文件路径
	mapsCache    map[uint32][]MemoryMapping // 进程内存映射缓存，key=PID
	languageCache map[uint32]string      // 进程语言检测缓存，key=PID
	goVersionCache map[uint32]string     // Go 版本信息缓存，key=PID
}

// elfFile 封装已解析的 ELF 文件信息
// 缓存符号表和调试信息，避免重复解析
type elfFile struct {
	file     *elf.File  // ELF 文件句柄
	symbols  []elf.Symbol // 符号表 (.symtab 或 .dynsym)
	dynSyms  []elf.Symbol // 动态符号表 (.dynsym)
	isGo     bool        // 是否为 Go 二进制
	goVersion string     // Go 版本信息
}

// ==================== 构造函数 ====================

// NewSymbolizer 创建一个新的多语言符号解析器
// 初始化缓存结构
func NewSymbolizer() *Symbolizer {
	return &Symbolizer{
		elfCache:      make(map[string]*elfFile),
		mapsCache:     make(map[uint32][]MemoryMapping),
		languageCache: make(map[uint32]string),
		goVersionCache: make(map[uint32]string),
	}
}

// ==================== 核心解析函数 ====================

// Resolve 将给定的虚拟地址解析为符号信息
// 参数:
//   - addr: 要解析的虚拟地址
//   - pid: 目标进程 ID
//
// 返回:
//   - name: 函数名/符号名
//   - file: 源代码文件路径
//   - line: 源代码行号
func (s *Symbolizer) Resolve(addr uint64, pid uint32) (string, string, int) {
	// 1. 获取进程的内存映射
	mappings := s.getProcessMaps(pid)
	if mappings == nil {
		return "", "", 0
	}

	// 2. 查找地址所在的内存映射区域
	for _, m := range mappings {
		if addr >= m.StartAddr && addr < m.EndAddr {
			// 3. 如果是匿名映射或无文件路径，返回地址信息
			if m.Pathname == "" || m.Pathname == "[anon:" {
				return fmt.Sprintf("[anon:0x%x]", addr), "", 0
			}

			// 4. 跳过特殊映射 (如 [vdso], [vvar], [heap] 等)
			if strings.HasPrefix(m.Pathname, "[") {
				return fmt.Sprintf("%s+0x%x", m.Pathname, addr-m.StartAddr), "", 0
			}

			// 5. 计算文件内偏移地址
			fileOffset := addr - m.StartAddr + m.Offset

			// 6. 根据语言类型选择解析策略
			lang := s.DetectLanguage(pid)
			switch lang {
			case "go":
				return s.resolveGoSymbol(m.Pathname, fileOffset, addr, pid)
			case "java":
				return s.resolveJavaSymbol(m.Pathname, fileOffset, addr)
			default:
				return s.resolveCSymbol(m.Pathname, fileOffset, addr)
			}
		}
	}

	// 地址不在任何映射区域中
	return fmt.Sprintf("[unknown:0x%x]", addr), "", 0
}

// DetectLanguage 检测目标进程的编程语言
// 通过分析 /proc/pid/maps 和二进制文件特征来判断
// 返回: "c" (C/C++) | "go" (Golang) | "java" (Java) | "unknown"
func (s *Symbolizer) DetectLanguage(pid uint32) string {
	s.mu.RLock()
	if lang, ok := s.languageCache[pid]; ok {
		s.mu.RUnlock()
		return lang
	}
	s.mu.RUnlock()

	// 获取进程内存映射
	mappings := s.getProcessMaps(pid)
	if mappings == nil {
		return "unknown"
	}

	// 检查是否为 Java 进程
	// Java 进程的特征: maps 中包含 libjvm.so
	for _, m := range mappings {
		if strings.Contains(m.Pathname, "libjvm.so") {
			s.mu.Lock()
			s.languageCache[pid] = "java"
			s.mu.Unlock()
			return "java"
		}
	}

	// 检查是否为 Go 进程
	// Go 进程的特征: maps 中包含 runtime 相关映射，或可执行文件包含 Go 特征
	for _, m := range mappings {
		if m.Pathname == "" {
			continue
		}
		// 检查可执行文件是否为 Go 二进制
		ef := s.loadELF(m.Pathname)
		if ef != nil && ef.isGo {
			s.mu.Lock()
			s.languageCache[pid] = "go"
			s.mu.Unlock()
			return "go"
		}
	}

	// 默认为 C/C++
	s.mu.Lock()
	s.languageCache[pid] = "c"
	s.mu.Unlock()
	return "c"
}

// ==================== C/C++ 符号解析 ====================

// resolveCSymbol 解析 C/C++ 程序的符号
// 通过读取 ELF 文件的 .symtab 和 .dynsym 符号表来查找地址对应的函数
func (s *Symbolizer) resolveCSymbol(filePath string, fileOffset uint64, addr uint64) (string, string, int) {
	// 加载 ELF 文件 (使用缓存)
	ef := s.loadELF(filePath)
	if ef == nil {
		// 无法解析 ELF 文件，返回文件名+偏移
		baseName := filepath.Base(filePath)
		return fmt.Sprintf("%s+0x%x", baseName, fileOffset), "", 0
	}

	// 优先从 .symtab (静态符号表) 中查找
	// .symtab 包含完整的调试符号信息
	name, file, line := s.findSymbolInTable(ef.symbols, fileOffset, filePath)
	if name != "" {
		return name, file, line
	}

	// 如果 .symtab 未找到，尝试 .dynsym (动态符号表)
	// .dynsym 包含动态链接的导出/导入符号
	name, file, line = s.findSymbolInTable(ef.dynSyms, fileOffset, filePath)
	if name != "" {
		return name, file, line
	}

	// 未找到符号，返回文件名+偏移
	baseName := filepath.Base(filePath)
	return fmt.Sprintf("%s+0x%x", baseName, fileOffset), "", 0
}

// findSymbolInTable 在符号表中查找指定偏移地址对应的符号
// 使用二分查找提高效率
func (s *Symbolizer) findSymbolInTable(symbols []elf.Symbol, offset uint64, filePath string) (string, string, int) {
	if len(symbols) == 0 {
		return "", "", 0
	}

	// 按地址排序的符号表中查找
	// 找到包含该地址的函数符号 (offset >= sym.Value && offset < sym.Value + sym.Size)
	var bestMatch elf.Symbol
	found := false

	for _, sym := range symbols {
		// 跳过非函数符号和非定义符号
		if sym.Size == 0 {
			continue
		}
		// STT_FUNC = 2, STT_GNU_IFUNC = 10
		if elf.ST_TYPE(sym.Info) != elf.STT_FUNC && elf.ST_TYPE(sym.Info) != 10 {
			continue
		}

		// 检查地址是否在符号范围内
		if offset >= sym.Value && offset < sym.Value+sym.Size {
			if !found || sym.Size < bestMatch.Size {
				// 选择范围最小的符号 (最精确匹配)
				bestMatch = sym
				found = true
			}
		}
	}

	if found {
		// 尝试从 DWARF 调试信息中获取文件名和行号
		// 注意: 标准 debug/elf 包不直接支持 DWARF 行号解析
		// 这里返回符号名和基础文件信息
		return bestMatch.Name, filePath, 0
	}

	return "", "", 0
}

// ==================== Golang 符号解析 ====================

// Go pclntab 相关常量
const (
	// Go 1.16+ 的 pclntab 魔数
	go16Magic  = 0xFFFFFFF1
	go118Magic = 0xFFFFFFF0
	go120Magic = 0xFFFFFFFA

	// Go buildinfo 魔数
	goBuildInfoMagic = "\xff Go buildinf:"
)

// resolveGoSymbol 解析 Go 程序的符号
// Go 使用 runtime.symtab (pclntab) 存储符号信息，格式与标准 ELF 不同
func (s *Symbolizer) resolveGoSymbol(filePath string, fileOffset uint64, addr uint64, pid uint32) (string, string, int) {
	// 加载 ELF 文件
	ef := s.loadELF(filePath)
	if ef == nil {
		baseName := filepath.Base(filePath)
		return fmt.Sprintf("%s+0x%x", baseName, fileOffset), "", 0
	}

	// 尝试从 Go pclntab 中解析符号
	name, file, line := s.resolveGoPclntab(ef, fileOffset)
	if name != "" {
		return name, file, line
	}

	// 回退到标准 ELF 符号表
	// Go 二进制也包含 ELF 符号表，但信息可能不完整
	name, file, line = s.findSymbolInTable(ef.symbols, fileOffset, filePath)
	if name != "" {
		return name, file, line
	}

	// 尝试动态符号表
	name, file, line = s.findSymbolInTable(ef.dynSyms, fileOffset, filePath)
	if name != "" {
		return name, file, line
	}

	baseName := filepath.Base(filePath)
	return fmt.Sprintf("%s+0x%x", baseName, fileOffset), "", 0
}

// resolveGoPclntab 从 Go 二进制的 pclntab 中解析符号
// pclntab 是 Go runtime 用于栈回溯和符号化的数据结构
// Go 1.18+ 使用新格式 (go12)
func (s *Symbolizer) resolveGoPclntab(ef *elfFile, offset uint64) (string, string, int) {
	// 查找 .gopclntab 段 (Go 1.2-1.15) 或 .go.buildinfo 段
	// Go 1.16+ 将 pclntab 存储在 .gopclntab 段中
	pclntabSection := ef.file.Section(".gopclntab")
	if pclntabSection == nil {
		// 尝试旧名称
		pclntabSection = ef.file.Section(".pclntab")
	}
	if pclntabSection == nil {
		return "", "", 0
	}

	// 读取 pclntab 数据
	data, err := pclntabSection.Data()
	if err != nil || len(data) < 16 {
		return "", "", 0
	}

	// 解析 pclntab 头部
	// Go 1.18+ 格式:
	//   - 8 字节魔数
	//   - 2 字节指令大小量子
	//   - 2 字节指针大小
	//   - nfunc (函数数量)
	//   - nfiles (文件数量)
	//   - ... 后续为函数表和文件表

	magic := binary.LittleEndian.Uint32(data[0:4])
	switch magic {
	case go16Magic, go118Magic, go120Magic:
		// Go 1.16+ 格式
		return s.parseGoPclntabNew(data, offset)
	default:
		// 旧格式或不识别
		return "", "", 0
	}
}

// parseGoPclntabNew 解析 Go 1.16+ 格式的 pclntab
// 在函数表中查找包含指定偏移的函数
func (s *Symbolizer) parseGoPclntabNew(data []byte, offset uint64) (string, string, int) {
	if len(data) < 16 {
		return "", "", 0
	}

	// 读取头部信息
	// 偏移 0: 魔数 (4字节)
	// 偏移 4: 指令大小量子 (2字节)
	// 偏移 6: 指针大小 (2字节)
	// 偏移 8: nfunc (4字节 或 8字节，取决于指针大小)
	// 偏移 12/16: nfiles

	ptrSize := int(binary.LittleEndian.Uint16(data[6:8]))
	if ptrSize != 4 && ptrSize != 8 {
		return "", "", 0
	}

	// 读取函数数量
	var nfunc int
	if ptrSize == 8 {
		nfunc = int(binary.LittleEndian.Uint64(data[8:16]))
	} else {
		nfunc = int(binary.LittleEndian.Uint32(data[8:12]))
	}

	// 函数表起始偏移 (头部大小)
	// Go 1.18+: header = 8 + 2 + 2 + ptrSize + ptrSize
	headerSize := 8 + 2 + 2 + ptrSize + ptrSize
	if headerSize >= len(data) {
		return "", "", 0
	}

	// 遍历函数表查找匹配的函数
	// 每个函数条目包含: funcPC, funcNameOff, ...
	// 注意: 实际的 pclntab 格式比较复杂，这里做简化处理
	// 在实际实现中，需要完整解析 Go 的 pclntab 格式

	// 由于完整解析 pclntab 非常复杂，这里使用简化策略:
	// 遍历函数表，每个条目固定大小 (取决于指针大小)
	entrySize := 2 * ptrSize // 简化的条目大小

	for i := 0; i < nfunc; i++ {
		entryOff := headerSize + i*entrySize
		if entryOff+entrySize > len(data) {
			break
		}

		// 读取函数起始 PC
		var funcPC uint64
		if ptrSize == 8 {
			funcPC = binary.LittleEndian.Uint64(data[entryOff : entryOff+8])
		} else {
			funcPC = uint64(binary.LittleEndian.Uint32(data[entryOff : entryOff+4]))
		}

		// 读取下一个函数的 PC 来确定当前函数的大小
		var nextPC uint64
		if i+1 < nfunc {
			nextEntryOff := headerSize + (i+1)*entrySize
			if nextEntryOff+ptrSize <= len(data) {
				if ptrSize == 8 {
					nextPC = binary.LittleEndian.Uint64(data[nextEntryOff : nextEntryOff+8])
				} else {
					nextPC = uint64(binary.LittleEndian.Uint32(data[nextEntryOff : nextEntryOff+4]))
				}
			}
		}

		// 检查偏移是否在当前函数范围内
		if offset >= funcPC && (nextPC == 0 || offset < nextPC) {
			// 读取函数名偏移
			var nameOff uint64
			if ptrSize == 8 {
				nameOff = binary.LittleEndian.Uint64(data[entryOff+8 : entryOff+16])
			} else {
				nameOff = uint64(binary.LittleEndian.Uint32(data[entryOff+4 : entryOff+8]))
			}

			// 从名称表中读取函数名
			funcName := s.readGoName(data, int(nameOff))
			if funcName != "" {
				return funcName, "", 0
			}
		}
	}

	return "", "", 0
}

// readGoName 从 Go pclntab 的名称表中读取函数名
// Go 的名称存储为以 null 结尾的字符串
func (s *Symbolizer) readGoName(data []byte, offset int) string {
	if offset < 0 || offset >= len(data) {
		return ""
	}

	// 查找 null 终止符
	end := offset
	for end < len(data) && data[end] != 0 {
		end++
	}

	if end == offset {
		return ""
	}

	return string(data[offset:end])
}

// ==================== Java 符号解析 ====================

// resolveJavaSymbol 解析 Java 程序的符号
// Java 的符号解析主要通过 libjvm.so 的 JNI 符号表来实现
func (s *Symbolizer) resolveJavaSymbol(filePath string, fileOffset uint64, addr uint64) (string, string, int) {
	// 加载 ELF 文件
	ef := s.loadELF(filePath)
	if ef == nil {
		baseName := filepath.Base(filePath)
		return fmt.Sprintf("%s+0x%x", baseName, fileOffset), "", 0
	}

	// 对于 Java 进程，主要解析 libjvm.so 中的 JNI 函数
	// JVM 的热点函数通常以 _ZN 开头 (C++ 修饰名) 或 Java_ 开头 (JNI 函数)
	name, file, line := s.findSymbolInTable(ef.symbols, fileOffset, filePath)
	if name != "" {
		// 尝试解码 C++ 修饰名 (简化处理)
		decodedName := s.demangleCppName(name)
		return decodedName, file, line
	}

	// 尝试动态符号表
	name, file, line = s.findSymbolInTable(ef.dynSyms, fileOffset, filePath)
	if name != "" {
		decodedName := s.demangleCppName(name)
		return decodedName, file, line
	}

	baseName := filepath.Base(filePath)
	return fmt.Sprintf("%s+0x%x", baseName, fileOffset), "", 0
}

// demangleCppName 简化的 C++ 名称解码
// 将 _ZN 开头的修饰名转换为可读形式
// 完整的 demangle 需要使用 libcxxabi 或类似库
func (s *Symbolizer) demangleCppName(name string) string {
	// 检查是否为 C++ 修饰名
	if !strings.HasPrefix(name, "_Z") {
		return name
	}

	// JNI 函数名格式: Java_<package>_<class>_<method>
	if strings.HasPrefix(name, "Java_") {
		return s.decodeJNIName(name)
	}

	// 简化的 C++ demangle
	// 实际项目中应使用 runtime/cgo 或外部 demangle 库
	return name
}

// decodeJNIName 解码 JNI 函数名
// JNI 函数名格式: Java_<package>_<class>_<method>
// 包名中的下划线 _ 替代了原始的 .
func (s *Symbolizer) decodeJNIName(name string) string {
	if !strings.HasPrefix(name, "Java_") {
		return name
	}

	// 去掉 "Java_" 前缀
	remaining := name[5:]

	// 将下划线分隔的部分转换为包名.类名.方法名
	parts := strings.Split(remaining, "_")
	if len(parts) < 2 {
		return name
	}

	// 最后一个部分是方法名
	methodName := parts[len(parts)-1]

	// 中间部分是包名和类名
	pkgAndClass := strings.Join(parts[:len(parts)-1], ".")

	return fmt.Sprintf("%s.%s", pkgAndClass, methodName)
}

// ==================== Go Buildinfo 解析 ====================

// GetGoVersion 获取 Go 程序的版本信息
// 通过读取 ELF 中的 .go.buildinfo 段或 runtime.buildinfo 来获取
func (s *Symbolizer) GetGoVersion(pid uint32) string {
	s.mu.RLock()
	if version, ok := s.goVersionCache[pid]; ok {
		s.mu.RUnlock()
		return version
	}
	s.mu.RUnlock()

	// 获取进程的可执行文件路径
	exePath := s.getProcessExePath(pid)
	if exePath == "" {
		return ""
	}

	// 加载 ELF 文件
	ef := s.loadELF(exePath)
	if ef == nil {
		return ""
	}

	// 查找 .go.buildinfo 段
	buildinfoSection := ef.file.Section(".go.buildinfo")
	if buildinfoSection == nil {
		return ""
	}

	// 读取 buildinfo 数据
	data, err := buildinfoSection.Data()
	if err != nil {
		return ""
	}

	// 解析 Go buildinfo
	// 格式: 魔数 (14字节) + 指针大小 (1字节) + ...
	if len(data) < len(goBuildInfoMagic)+1 {
		return ""
	}

	// 验证魔数
	magic := string(data[:len(goBuildInfoMagic)])
	if magic != goBuildInfoMagic {
		return ""
	}

	// 读取指针大小
	ptrSize := int(data[len(goBuildInfoMagic)])

	// 读取 Go 版本字符串
	// buildinfo 中包含多个以 null 分隔的字符串对 (key, value)
	// 第一个字符串对通常是 ("Go", "go1.x.x")
	strOffset := len(goBuildInfoMagic) + 1 + ptrSize*2 + 8 // 跳过头部

	// 读取字符串表
	strings_ := s.readBuildinfoStrings(data, strOffset, ptrSize)
	for i := 0; i+1 < len(strings_); i += 2 {
		if strings_[i] == "Go" {
			// 缓存并返回版本信息
			s.mu.Lock()
			s.goVersionCache[pid] = strings_[i+1]
			s.mu.Unlock()
			return strings_[i+1]
		}
	}

	return ""
}

// readBuildinfoStrings 从 Go buildinfo 数据中读取字符串表
func (s *Symbolizer) readBuildinfoStrings(data []byte, offset int, ptrSize int) []string {
	var result []string

	for offset < len(data) {
		// 读取字符串长度 (指针大小)
		if offset+ptrSize > len(data) {
			break
		}

		var strLen int
		if ptrSize == 8 {
			strLen = int(binary.LittleEndian.Uint64(data[offset : offset+8]))
		} else {
			strLen = int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		}
		offset += ptrSize

		// 读取字符串数据
		if offset+strLen > len(data) {
			break
		}

		str := string(data[offset : offset+strLen])
		result = append(result, str)
		offset += strLen

		// 空字符串表示结束
		if strLen == 0 {
			break
		}
	}

	return result
}

// ==================== /proc 文件系统解析 ====================

// getProcessMaps 读取进程的内存映射信息
// 解析 /proc/pid/maps 文件，返回内存映射列表
// 结果会被缓存以提高性能
func (s *Symbolizer) getProcessMaps(pid uint32) []MemoryMapping {
	// 检查缓存
	s.mu.RLock()
	if maps, ok := s.mapsCache[pid]; ok {
		s.mu.RUnlock()
		return maps
	}
	s.mu.RUnlock()

	// 打开 /proc/pid/maps 文件
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)
	file, err := os.Open(mapsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var mappings []MemoryMapping
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		mapping, ok := parseMapsLine(line)
		if ok {
			mappings = append(mappings, mapping)
		}
	}

	// 缓存结果
	s.mu.Lock()
	s.mapsCache[pid] = mappings
	s.mu.Unlock()

	return mappings
}

// parseMapsLine 解析 /proc/pid/maps 文件的一行
// 格式: 地址范围 权限 偏移 设备 inode 路径名
// 示例: 7f8b0a000000-7f8b0a021000 r-xp 00000000 08:01 12345 /usr/lib/libc.so.6
func parseMapsLine(line string) (MemoryMapping, bool) {
	// 使用正则表达式解析 maps 行
	// 格式: start-end perms offset dev inode pathname
	re := regexp.MustCompile(`^([0-9a-f]+)-([0-9a-f]+)\s+([rwxsp-]+)\s+([0-9a-f]+)\s+([0-9a-f:]+)\s+(\d+)\s*(.*)?$`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return MemoryMapping{}, false
	}

	startAddr, err := strconv.ParseUint(matches[1], 16, 64)
	if err != nil {
		return MemoryMapping{}, false
	}

	endAddr, err := strconv.ParseUint(matches[2], 16, 64)
	if err != nil {
		return MemoryMapping{}, false
	}

	offset, err := strconv.ParseUint(matches[4], 16, 64)
	if err != nil {
		return MemoryMapping{}, false
	}

	inode, err := strconv.ParseUint(matches[6], 10, 64)
	if err != nil {
		return MemoryMapping{}, false
	}

	return MemoryMapping{
		StartAddr:   startAddr,
		EndAddr:     endAddr,
		Permissions: matches[3],
		Offset:      offset,
		Device:      matches[5],
		Inode:       inode,
		Pathname:    strings.TrimSpace(matches[7]),
	}, true
}

// getProcessExePath 获取进程的可执行文件路径
// 通过读取 /proc/pid/exe 符号链接获取
func (s *Symbolizer) getProcessExePath(pid uint32) string {
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	realPath, err := os.Readlink(exePath)
	if err != nil {
		return ""
	}
	return realPath
}

// ==================== ELF 文件加载与缓存 ====================

// loadELF 加载 ELF 文件并缓存
// 如果文件已在缓存中，直接返回缓存的结果
func (s *Symbolizer) loadELF(filePath string) *elfFile {
	// 规范化文件路径
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	// 检查缓存
	s.mu.RLock()
	if ef, ok := s.elfCache[absPath]; ok {
		s.mu.RUnlock()
		return ef
	}
	s.mu.RUnlock()

	// 打开 ELF 文件
	file, err := elf.Open(absPath)
	if err != nil {
		return nil
	}

	ef := &elfFile{
		file: file,
	}

	// 读取 .symtab (静态符号表)
	if symtabSection := file.Section(".symtab"); symtabSection != nil {
		symbols, err := file.Symbols()
		if err == nil {
			ef.symbols = symbols
		}
	}

	// 读取 .dynsym (动态符号表)
	if dynsymSection := file.Section(".dynsym"); dynsymSection != nil {
		dynSyms, err := file.DynamicSymbols()
		if err == nil {
			ef.dynSyms = dynSyms
		}
	}

	// 检测是否为 Go 二进制
	ef.isGo = s.detectGoBinary(file)

	// 缓存结果
	s.mu.Lock()
	s.elfCache[absPath] = ef
	s.mu.Unlock()

	return ef
}

// detectGoBinary 检测 ELF 文件是否为 Go 编译的二进制
// 通过检查 .gopclntab 段或 .note.go.buildid 段来判断
func (s *Symbolizer) detectGoBinary(file *elf.File) bool {
	// 检查 .gopclntab 段
	if file.Section(".gopclntab") != nil {
		return true
	}

	// 检查 .note.go.buildid 段
	if file.Section(".note.go.buildid") != nil {
		return true
	}

	// 检查 .go.buildinfo 段
	if file.Section(".go.buildinfo") != nil {
		return true
	}

	// 检查 .rodata 段中是否包含 Go buildinfo 魔数
	rodata := file.Section(".rodata")
	if rodata != nil {
		data, err := rodata.Data()
		if err == nil {
			if len(data) > len(goBuildInfoMagic) {
				if string(data[:len(goBuildInfoMagic)]) == goBuildInfoMagic {
					return true
				}
			}
		}
	}

	return false
}

// ==================== 缓存管理 ====================

// ClearCache 清除所有缓存
// 当进程状态发生变化时调用
func (s *Symbolizer) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有缓存的 ELF 文件句柄
	for path, ef := range s.elfCache {
		if ef.file != nil {
			ef.file.Close()
		}
		delete(s.elfCache, path)
	}

	// 清除其他缓存
	s.mapsCache = make(map[uint32][]MemoryMapping)
	s.languageCache = make(map[uint32]string)
	s.goVersionCache = make(map[uint32]string)
}

// ClearProcessCache 清除指定进程的缓存
func (s *Symbolizer) ClearProcessCache(pid uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.mapsCache, pid)
	delete(s.languageCache, pid)
	delete(s.goVersionCache, pid)
}

// InvalidateMapsCache 使指定进程的内存映射缓存失效
// 当检测到进程加载了新的共享库时调用
func (s *Symbolizer) InvalidateMapsCache(pid uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mapsCache, pid)
}
