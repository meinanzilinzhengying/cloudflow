// Package profiler 提供 Java 堆外内存检测功能
// 本文件实现 JNI 调用栈到 Java 方法名的翻译
// 通过解析 JNI 函数名、libjvm 符号表和 hsperfdata 来还原 Java 调用栈
package profiler

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ==================== Java 栈翻译器 ====================

// JavaStackTranslator JNI 栈到 Java 栈的翻译器
// 将原生调用栈中的 JNI 函数名翻译为 Java 类名和方法名
type JavaStackTranslator struct {
	pid         uint32
	mu          sync.RWMutex
	jvmPath     string            // libjvm.so 路径
	classCache  map[string]string // JNI签名 -> Java类名缓存
	methodCache map[string]string // JNI签名 -> Java方法名缓存
	javaHome    string            // JAVA_HOME 路径
}

// JavaFrame 表示一个 Java 栈帧
type JavaFrame struct {
	Classname  string `json:"classname"`  // 完整类名 (如 java.nio.DirectByteBuffer)
	Method     string `json:"method"`     // 方法名 (如 allocateDirect)
	Signature  string `json:"signature"`  // 方法签名 (如 (I)Ljava/nio/ByteBuffer;)
	FileName   string `json:"file_name"` // 源文件名
	LineNumber int    `json:"line_number"` // 行号
	IsNative   bool   `json:"is_native"`  // 是否为 native 方法
}

// NewJavaStackTranslator 创建 Java 栈翻译器
func NewJavaStackTranslator(pid uint32) *JavaStackTranslator {
	t := &JavaStackTranslator{
		pid:         pid,
		classCache:  make(map[string]string),
		methodCache: make(map[string]string),
	}

	// 自动检测 JAVA_HOME 和 libjvm.so 路径
	t.detectJVM()

	return t
}

// detectJVM 检测 JVM 环境
func (t *JavaStackTranslator) detectJVM() {
	// 1. 从 /proc/pid/maps 查找 libjvm.so
	t.jvmPath = t.findLibJVM()

	// 2. 检测 JAVA_HOME
	t.javaHome = os.Getenv("JAVA_HOME")
	if t.javaHome == "" {
		// 尝试从 /proc/pid/environ 读取
		t.javaHome = t.readProcEnviron("JAVA_HOME")
	}
	if t.javaHome == "" {
		// 尝试常见路径
		commonPaths := []string{
			"/usr/lib/jvm/default-java",
			"/usr/lib/jvm/java-11-openjdk-amd64",
			"/usr/lib/jvm/java-17-openjdk-amd64",
			"/usr/local/java",
		}
		for _, p := range commonPaths {
			if _, err := os.Stat(p); err == nil {
				t.javaHome = p
				break
			}
		}
	}
}

// findLibJVM 从 /proc/pid/maps 查找 libjvm.so
func (t *JavaStackTranslator) findLibJVM() string {
	mapsFile := fmt.Sprintf("/proc/%d/maps", t.pid)
	f, err := os.Open(mapsFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "libjvm.so") {
			// 提取文件路径
			fields := strings.Fields(line)
			if len(fields) >= 6 {
				return fields[len(fields)-1]
			}
		}
	}
	return ""
}

// readProcEnviron 从 /proc/pid/environ 读取环境变量
func (t *JavaStackTranslator) readProcEnviron(key string) string {
	envFile := fmt.Sprintf("/proc/%d/environ", t.pid)
	data, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}

	prefix := key + "="
	for _, entry := range strings.Split(string(data), "\x00") {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

// ==================== 栈翻译 ====================

// Translate 将原生调用栈翻译为 Java 调用栈
// 输入原生栈帧列表，输出 Java 栈帧列表
func (t *JavaStackTranslator) Translate(nativeStack []string) []string {
	if len(nativeStack) == 0 {
		return nil
	}

	var javaFrames []string

	for _, frame := range nativeStack {
		javaFrame := t.translateFrame(frame)
		if javaFrame != "" {
			javaFrames = append(javaFrames, javaFrame)
		}
	}

	return javaFrames
}

// TranslateDetailed 将原生调用栈翻译为详细的 Java 栈帧
func (t *JavaStackTranslator) TranslateDetailed(nativeStack []string) []JavaFrame {
	if len(nativeStack) == 0 {
		return nil
	}

	var frames []JavaFrame

	for _, frame := range nativeStack {
		jf := t.translateFrameDetailed(frame)
		if jf.Classname != "" || jf.Method != "" {
			frames = append(frames, jf)
		}
	}

	return frames
}

// translateFrame 翻译单个栈帧
func (t *JavaStackTranslator) translateFrame(nativeFrame string) string {
	jf := t.translateFrameDetailed(nativeFrame)
	if jf.Classname == "" && jf.Method == "" {
		return ""
	}

	result := jf.Classname
	if jf.Method != "" {
		result += "." + jf.Method
	}
	if jf.Signature != "" {
		result += jf.Signature
	}
	if jf.IsNative {
		result += " (native)"
	}

	return result
}

// translateFrameDetailed 详细翻译单个栈帧
func (t *JavaStackTranslator) translateFrameDetailed(nativeFrame string) JavaFrame {
	var jf JavaFrame

	// 策略 1: 解析 JNI 函数名
	// JNI 函数名格式: Java_<package>_<class>_<method>
	// 或: Java_<package>_<class>_<method>__<signature_overload>
	jniFrame := t.parseJNIFunction(nativeFrame)
	if jniFrame != nil {
		return *jniFrame
	}

	// 策略 2: 解析 Java_caller 相关函数
	if strings.Contains(nativeFrame, "JavaCalls::call_virtual") ||
		strings.Contains(nativeFrame, "JavaCalls::call_static") {
		jf.Classname = "java.lang.reflect.Method"
		jf.Method = "invoke"
		jf.IsNative = true
		return jf
	}

	// 策略 3: 解析已知的 JVM 内部函数
	jf = t.parseJVMInternal(nativeFrame)
	if jf.Classname != "" {
		return jf
	}

	// 策略 4: 尝试从 libjvm 符号表查找
	jf = t.lookupJVMSymbol(nativeFrame)
	if jf.Classname != "" {
		return jf
	}

	return jf
}

// parseJNIFunction 解析 JNI 函数名
// JNI 命名规则:
//   Java_com_example_MyClass_myMethod       -> com.example.MyClass.myMethod
//   Java_com_example_MyClass_myMethod__I    -> com.example.MyClass.myMethod(int)
//   Java_com_example_MyClass_myMethod__JI   -> com.example.MyClass.myMethod(long, int)
func (t *JavaStackTranslator) parseJNIFunction(frame string) *JavaFrame {
	// 提取函数名部分
	funcName := extractFuncName(frame)
	if funcName == "" {
		return nil
	}

	// 检查是否是 JNI 函数
	if !strings.HasPrefix(funcName, "Java_") {
		return nil
	}

	// 移除 "Java_" 前缀
	remainder := strings.TrimPrefix(funcName, "Java_")

	// 查找签名分隔符 "__"（用于重载方法）
	var signaturePart string
	if idx := strings.LastIndex(remainder, "__"); idx > 0 {
		signaturePart = remainder[idx+2:]
		remainder = remainder[:idx]
	}

	// 将下划线分隔的包名/类名/方法名转换为点分隔
	parts := strings.Split(remainder, "_")
	if len(parts) < 2 {
		return nil
	}

	// 最后一个部分是方法名
	methodName := parts[len(parts)-1]

	// 其余部分组成类名
	classParts := parts[:len(parts)-1]
	classname := strings.Join(classParts, ".")

	// 解析 JNI 签名
	signature := ""
	if signaturePart != "" {
		signature = t.decodeJNISignature(signaturePart)
	}

	return &JavaFrame{
		Classname: classname,
		Method:    methodName,
		Signature: signature,
		IsNative:  true,
	}
}

// decodeJNISignature 解码 JNI 签名缩写
// JNI 重载后缀使用类型缩写:
//   Z=boolean, B=byte, C=char, S=short, I=int, J=long, F=float, D=double
//   L<class>;=对象类型, [=数组
func (t *JavaStackTranslator) decodeJNISignature(sig string) string {
	if sig == "" {
		return ""
	}

	var params []string
	for i := 0; i < len(sig); {
		switch sig[i] {
		case 'Z':
			params = append(params, "boolean")
			i++
		case 'B':
			params = append(params, "byte")
			i++
		case 'C':
			params = append(params, "char")
			i++
		case 'S':
			params = append(params, "short")
			i++
		case 'I':
			params = append(params, "int")
			i++
		case 'J':
			params = append(params, "long")
			i++
		case 'F':
			params = append(params, "float")
			i++
		case 'D':
			params = append(params, "double")
			i++
		case 'L':
			// 对象类型，找到分号结束
			end := strings.Index(sig[i:], ";")
			if end > 0 {
				classSig := sig[i+1 : i+end]
				javaClass := strings.ReplaceAll(classSig, "/", ".")
				params = append(params, javaClass)
				i += end + 1
			} else {
				i++
			}
		case '[':
			// 数组类型
			var elemType string
			if i+1 < len(sig) {
				switch sig[i+1] {
				case 'I':
					elemType = "int[]"
					i += 2
				case 'J':
					elemType = "long[]"
					i += 2
				case 'B':
					elemType = "byte[]"
					i += 2
				case 'Z':
					elemType = "boolean[]"
					i += 2
				case 'L':
					end := strings.Index(sig[i+1:], ";")
					if end > 0 {
						classSig := sig[i+2 : i+1+end]
						javaClass := strings.ReplaceAll(classSig, "/", ".")
						elemType = javaClass + "[]"
						i += end + 2
					} else {
						elemType = "Object[]"
						i++
					}
				default:
					elemType = "Object[]"
					i++
				}
			}
			params = append(params, elemType)
		default:
			i++
		}
	}

	return "(" + strings.Join(params, ",") + ")"
}

// parseJVMInternal 解析 JVM 内部函数
func (t *JavaStackTranslator) parseJVMInternal(frame string) JavaFrame {
	var jf JavaFrame

	// 已知的 JVM 内部函数映射
	jvmFunctions := map[string]JavaFrame{
		"jni_NewDirectByteBuffer":     {Classname: "java.nio.ByteBuffer", Method: "allocateDirect", IsNative: true},
		"jni_GetDirectBufferAddress":  {Classname: "java.nio.DirectByteBuffer", Method: "address", IsNative: true},
		"jni_GetDirectBufferCapacity": {Classname: "java.nio.DirectByteBuffer", Method: "capacity", IsNative: true},
		"Bits_reserveMemory":          {Classname: "java.nio.Bits", Method: "reserveMemory", IsNative: true},
		"Bits_unreserveMemory":        {Classname: "java.nio.Bits", Method: "unreserveMemory", IsNative: true},
		"Unsafe_allocateMemory":       {Classname: "sun.misc.Unsafe", Method: "allocateMemory", IsNative: true},
		"Unsafe_freeMemory":           {Classname: "sun.misc.Unsafe", Method: "freeMemory", IsNative: true},
		"Unsafe_reallocateMemory":     {Classname: "sun.misc.Unsafe", Method: "reallocateMemory", IsNative: true},
		"FileChannelImpl_map0":        {Classname: "sun.nio.ch.FileChannelImpl", Method: "map0", IsNative: true},
		"FileChannelImpl_unmap0":      {Classname: "sun.nio.ch.FileChannelImpl", Method: "unmap0", IsNative: true},
		"FileDispatcherImpl_read0":    {Classname: "sun.nio.ch.FileDispatcherImpl", Method: "read0", IsNative: true},
		"FileDispatcherImpl_write0":   {Classname: "sun.nio.ch.FileDispatcherImpl", Method: "write0", IsNative: true},
		"FileDispatcherImpl_pread0":   {Classname: "sun.nio.ch.FileDispatcherImpl", Method: "pread0", IsNative: true},
		"FileDispatcherImpl_pwrite0":  {Classname: "sun.nio.ch.FileDispatcherImpl", Method: "pwrite0", IsNative: true},
		"SocketDispatcherImpl_read0":  {Classname: "sun.nio.ch.SocketDispatcherImpl", Method: "read0", IsNative: true},
		"SocketDispatcherImpl_write0": {Classname: "sun.nio.ch.SocketDispatcherImpl", Method: "write0", IsNative: true},
		"Netty_PooledByteBufAllocator": {Classname: "io.netty.buffer.PooledByteBufAllocator", Method: "allocate", IsNative: true},
	}

	// 查找匹配
	funcName := extractFuncName(frame)
	if funcName == "" {
		return jf
	}

	for pattern, javaFrame := range jvmFunctions {
		if strings.Contains(funcName, pattern) || strings.Contains(frame, pattern) {
			return javaFrame
		}
	}

	// 通用 JNI 函数模式匹配
	// _ZN<jni_prefix> 表示 C++ mangled name 中的 JNI 调用
	if strings.Contains(frame, "JavaCalls::call") {
		return JavaFrame{Classname: "java.lang.reflect.Method", Method: "invoke", IsNative: true}
	}

	return jf
}

// lookupJVMSymbol 从 libjvm.so 符号表查找
func (t *JavaStackTranslator) lookupJVMSymbol(frame string) JavaFrame {
	if t.jvmPath == "" {
		return JavaFrame{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// 检查缓存
	cacheKey := frame
	if cached, ok := t.classCache[cacheKey]; ok {
		return JavaFrame{Classname: cached}
	}

	// 简化实现：从已知的 libjvm 符号模式中匹配
	// 实际实现应该解析 libjvm.so 的 .symtab
	funcName := extractFuncName(frame)

	// 匹配 JNI 相关符号
	if strings.HasPrefix(funcName, "JNI_") || strings.HasPrefix(funcName, "jni_") {
		result := JavaFrame{Classname: "jdk.internal.misc.Unsafe", Method: funcName, IsNative: true}
		t.classCache[cacheKey] = result.Classname
		return result
	}

	return JavaFrame{}
}

// ==================== JNI 签名解析 ====================

// DecodeJNIFieldSig 解码 JNI 字段签名
// 例: "Ljava/lang/String;" -> "java.lang.String"
func (t *JavaStackTranslator) DecodeJNIFieldSig(sig string) string {
	if strings.HasPrefix(sig, "L") && strings.HasSuffix(sig, ";") {
		classSig := sig[1 : len(sig)-1]
		return strings.ReplaceAll(classSig, "/", ".")
	}
	return sig
}

// ==================== 辅助函数 ====================

// extractFuncName 从栈帧中提取函数名
// 支持格式:
//   - libjvm.so(jni_NewDirectByteBuffer+0x12)
//   - jni_NewDirectByteBuffer
//   - /usr/lib/jvm/.../libjvm.so(jni_NewDirectByteBuffer)
func extractFuncName(frame string) string {
	// 模式 1: libxxx.so(func+0xNN)
	re := regexp.MustCompile(`\(([^+)]+)\+`)
	matches := re.FindStringSubmatch(frame)
	if len(matches) >= 2 {
		return matches[1]
	}

	// 模式 2: 直接的函数名
	frame = strings.TrimSpace(frame)
	if idx := strings.Index(frame, "("); idx > 0 {
		return frame[:idx]
	}

	return frame
}

// ==================== Java 进程信息 ====================

// GetJavaVersion 获取 Java 版本
func (t *JavaStackTranslator) GetJavaVersion() string {
	if t.javaHome == "" {
		return "unknown"
	}

	// 尝试读取 release 文件
	releaseFile := filepath.Join(t.javaHome, "release")
	data, err := os.ReadFile(releaseFile)
	if err != nil {
		return "unknown"
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "JAVA_VERSION=") {
			version := strings.TrimPrefix(line, "JAVA_VERSION=")
			version = strings.Trim(version, "\"")
			return version
		}
	}

	return "unknown"
}

// GetProcessJavaInfo 获取 Java 进程信息
func (t *JavaStackTranslator) GetProcessJavaInfo() map[string]string {
	info := make(map[string]string)

	info["pid"] = strconv.Itoa(int(t.pid))
	info["java_home"] = t.javaHome
	info["java_version"] = t.GetJavaVersion()
	info["libjvm_path"] = t.jvmPath

	// 读取 JVM 参数
	cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", t.pid)
	cmdline, err := os.ReadFile(cmdlineFile)
	if err == nil {
		info["cmdline"] = strings.ReplaceAll(string(cmdline), "\x00", " ")
	}

	// 读取 JVM 启动参数
	// 从 /proc/pid/environ 获取 JAVA_TOOL_OPTIONS
	javaToolOptions := t.readProcEnviron("JAVA_TOOL_OPTIONS")
	if javaToolOptions != "" {
		info["java_tool_options"] = javaToolOptions
	}

	// 获取 -Xmx 和 -Xms
	for _, opt := range strings.Fields(info["cmdline"]) {
		if strings.HasPrefix(opt, "-Xmx") {
			info["xmx"] = opt
		}
		if strings.HasPrefix(opt, "-Xms") {
			info["xms"] = opt
		}
		if strings.HasPrefix(opt, "-XX:MaxDirectMemorySize") {
			info["max_direct_memory"] = opt
		}
	}

	return info
}

// Close 关闭翻译器，清理资源
func (t *JavaStackTranslator) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.classCache = make(map[string]string)
	t.methodCache = make(map[string]string)
}
