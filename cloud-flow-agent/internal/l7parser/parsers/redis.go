// Package parsers Redis 协议解析器 (RESP)
//
// RESP (REdis Serialization Protocol) 规范:
//   - Simple Strings: +<string>\r\n
//   - Errors: -<error>\r\n
//   - Integers: :<number>\r\n
//   - Bulk Strings: $<length>\r\n<data>\r\n
//   - Arrays: *<count>\r\n<elements...>
//
// 实现要点:
//   - 支持 RESP2 和 RESP3
//   - 流式解析，处理部分包
//   - 命令识别和参数提取

package parsers

import (
	"bytes"
	"context"
	"errors"
	"strconv"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidRESP     = errors.New("invalid RESP format")
	ErrIncompleteRESP  = errors.New("incomplete RESP data")
)

// RESP 类型常量
const (
	RESPString    = '+'
	RESPError     = '-'
	RESPInteger   = ':'
	RESPBulk      = '$'
	RESPArray     = '*'
	RESPNull      = '_'
	RESPFloat     = ','
	RESPBool      = '#'
	RESPBignum    = '('
	RESPPush      = '>'
	RESPSet       = '~'
	RESPMap       = '%'
	RESPAttribute = '|'
)

// RedisCommand Redis 命令信息
type RedisCommand struct {
	Name      string
	Args      []string
	ArgCount  int
	IsInline  bool // 是否是内联命令 (telnet 风格)
}

// RedisParser Redis 协议解析器
type RedisParser struct {
	// 解析状态
	state *redisParseState
}

// redisParseState 解析状态 (用于流式解析)
type redisParseState struct {
	// 缓冲区
	buffer []byte

	// 当前解析位置
	offset int

	// 解析阶段
	phase parsePhase

	// 当前命令
	currentCmd *RedisCommand

	// 数组解析状态
	arrayDepth int
	arrayCounts []int
}

type parsePhase uint8

const (
	phaseIdle parsePhase = iota
	phaseReadingType
	phaseReadingBulkLen
	phaseReadingBulkData
	phaseReadingArray
)

// NewRedisParser 创建 Redis 解析器
func NewRedisParser() *RedisParser {
	return &RedisParser{
		state: &redisParseState{
			buffer:      make([]byte, 0, 4096),
			arrayCounts: make([]int, 0, 8),
		},
	}
}

// Type 返回解析器类型
func (p *RedisParser) Type() l7parser.ParserType {
	return l7parser.ParserTypeRedis
}

// Name 返回解析器名称
func (p *RedisParser) Name() string {
	return "redis"
}

// Priority 返回解析优先级
func (p *RedisParser) Priority() int {
	return 70
}

// Detect 协议检测
func (p *RedisParser) Detect(data []byte, dstPort uint16) (bool, float64) {
	if len(data) == 0 {
		return false, 0
	}

	first := data[0]

	// 检查 RESP 类型标识
	switch first {
	case RESPString, RESPError, RESPInteger, RESPBulk, RESPArray:
		// 标准 RESP
		if dstPort == 6379 || dstPort == 6380 {
			return true, 0.95
		}
		return true, 0.85

	case RESPNull, RESPFloat, RESPBool, RESPBignum, RESPPush, RESPSet, RESPMap, RESPAttribute:
		// RESP3 类型
		if dstPort == 6379 || dstPort == 6380 {
			return true, 0.90
		}
		return true, 0.80
	}

	// 检查内联命令 (telnet 风格)
	// 例如: "GET key\r\n", "SET key value\r\n"
	if isInlineCommand(data) {
		if dstPort == 6379 || dstPort == 6380 {
			return true, 0.90
		}
		return true, 0.75
	}

	return false, 0
}

// isInlineCommand 检查是否为内联命令
func isInlineCommand(data []byte) bool {
	// 内联命令以可打印字符开头，以 \r\n 结尾
	// 且包含空格分隔的参数
	if len(data) < 3 {
		return false
	}

	// 检查是否以 RESP 类型字符开头
	if bytes.IndexByte([]byte("+-:$*_#,(>~%|"), data[0]) >= 0 {
		return false
	}

	// 查找 \r\n
	if !bytes.Contains(data, []byte("\r\n")) {
		return false
	}

	// 检查是否包含空格 (命令和参数的分隔)
	return bytes.Contains(data, []byte(" "))
}

// Parse 解析数据包
func (p *RedisParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	data := input.Packet.Data
	if len(data) == 0 {
		return nil, state, nil
	}

	// 获取或创建解析状态
	parseState, ok := state.(*redisParseState)
	if !ok {
		parseState = &redisParseState{
			buffer:      make([]byte, 0, 4096),
			arrayCounts: make([]int, 0, 8),
		}
	}

	// 添加数据到缓冲区
	parseState.buffer = append(parseState.buffer, data...)

	result := &l7parser.ParseResult{
		ParserType: l7parser.ParserTypeRedis,
		Headers:    make(map[string]string),
	}

	// 解析命令
	cmd, err := p.parseCommand(parseState)
	if err != nil {
		if err == ErrIncompleteRESP {
			result.IsPartial = true
			result.NeedMore = true
			return result, parseState, nil
		}
		return nil, parseState, err
	}

	if cmd != nil {
		result.Direction = l7parser.DirRequest
		result.Headers["command"] = cmd.Name
		if len(cmd.Args) > 0 {
			result.Headers["key"] = cmd.Args[0]
		}
		result.ReqSize = uint32(len(data))
		result.IsComplete = true
	}

	return result, parseState, nil
}

// parseCommand 解析 Redis 命令
func (p *RedisParser) parseCommand(state *redisParseState) (*RedisCommand, error) {
	if len(state.buffer) == 0 {
		return nil, ErrIncompleteRESP
	}

	// 检查是否为内联命令
	if isInlineCommand(state.buffer) {
		return p.parseInlineCommand(state)
	}

	// 解析 RESP 数组 (命令格式: *2\r\n$4\r\nLLEN\r\n$6\r\nmylist\r\n)
	return p.parseRESPArrayCommand(state)
}

// parseInlineCommand 解析内联命令
func (p *RedisParser) parseInlineCommand(state *redisParseState) (*RedisCommand, error) {
	// 查找 \r\n
	endIdx := bytes.Index(state.buffer, []byte("\r\n"))
	if endIdx < 0 {
		return nil, ErrIncompleteRESP
	}

	// 分割参数
	line := string(state.buffer[:endIdx])
	parts := bytes.Fields(state.buffer[:endIdx])

	if len(parts) == 0 {
		return nil, ErrInvalidRESP
	}

	cmd := &RedisCommand{
		Name:     string(bytes.ToUpper(parts[0])),
		IsInline: true,
		ArgCount: len(parts) - 1,
	}

	if len(parts) > 1 {
		cmd.Args = make([]string, len(parts)-1)
		for i, arg := range parts[1:] {
			cmd.Args[i] = string(arg)
		}
	}

	// 消费数据
	state.buffer = state.buffer[endIdx+2:]

	return cmd, nil
}

// parseRESPArrayCommand 解析 RESP 数组格式的命令
func (p *RedisParser) parseRESPArrayCommand(state *redisParseState) (*RedisCommand, error) {
	// 检查数组开始
	if state.buffer[0] != RESPArray {
		// 可能是响应，不是命令
		return nil, nil
	}

	// 解析数组元素个数
	count, offset, err := p.readInteger(state.buffer, 1)
	if err != nil {
		return nil, err
	}

	if count <= 0 {
		return nil, ErrInvalidRESP
	}

	// 解析数组元素 (应该是 bulk strings)
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if offset >= len(state.buffer) {
			return nil, ErrIncompleteRESP
		}

		// 检查 bulk string 类型
		if state.buffer[offset] != RESPBulk {
			return nil, ErrInvalidRESP
		}

		// 解析 bulk string 长度
		length, newOffset, err := p.readInteger(state.buffer, offset+1)
		if err != nil {
			return nil, err
		}
		offset = newOffset

		if length < 0 {
			// null bulk string
			args = append(args, "")
			continue
		}

		// 检查数据是否完整
		if offset+length+2 > len(state.buffer) {
			return nil, ErrIncompleteRESP
		}

		// 读取数据
		arg := string(state.buffer[offset : offset+length])
		args = append(args, arg)
		offset += length + 2 // +2 for \r\n
	}

	if len(args) == 0 {
		return nil, ErrInvalidRESP
	}

	cmd := &RedisCommand{
		Name:     string(bytes.ToUpper([]byte(args[0]))),
		Args:     args[1:],
		ArgCount: len(args) - 1,
		IsInline: false,
	}

	// 消费数据
	state.buffer = state.buffer[offset:]

	return cmd, nil
}

// readInteger 读取 RESP 整数
func (p *RedisParser) readInteger(data []byte, offset int) (int, int, error) {
	// 查找 \r\n
	endIdx := bytes.Index(data[offset:], []byte("\r\n"))
	if endIdx < 0 {
		return 0, 0, ErrIncompleteRESP
	}

	// 解析整数
	numStr := string(data[offset : offset+endIdx])
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, 0, ErrInvalidRESP
	}

	return num, offset + endIdx + 2, nil
}

// ParseStreaming 流式解析
func (p *RedisParser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *RedisParser) Reset() {
	p.state = &redisParseState{
		buffer:      make([]byte, 0, 4096),
		arrayCounts: make([]int, 0, 8),
	}
}

// ============================================================================
// 常用 Redis 命令
// ============================================================================

// IsReadCommand 检查是否为读命令
func IsReadCommand(cmd string) bool {
	readCmds := map[string]bool{
		"GET": true, "MGET": true, "GETRANGE": true, "GETBIT": true,
		"STRLEN": true, "EXISTS": true, "TTL": true, "PTTL": true,
		"KEYS": true, "SCAN": true, "RANDOMKEY": true, "TYPE": true,
		"LRANGE": true, "LLEN": true, "LINDEX": true, "LPOS": true,
		"SCARD": true, "SISMEMBER": true, "SMEMBERS": true, "SRANDMEMBER": true,
		"SSCAN": true, "SUNION": true, "SINTER": true, "SDIFF": true,
		"ZCARD": true, "ZCOUNT": true, "ZRANGE": true, "ZREVRANGE": true,
		"ZRANGEBYSCORE": true, "ZREVRANGEBYSCORE": true, "ZRANK": true,
		"ZREVRANK": true, "ZSCORE": true, "ZSCAN": true, "ZLEXCOUNT": true,
		"ZRANGEBYLEX": true, "HGET": true, "HMGET": true, "HGETALL": true,
		"HKEYS": true, "HVALS": true, "HLEN": true, "HEXISTS": true,
		"HSCAN": true, "HSTRLEN": true, "PFCOUNT": true, "PING": true,
		"ECHO": true, "TIME": true, "INFO": true, "CONFIG": true,
		"MONITOR": true, "SLOWLOG": true, "CLIENT": true, "CLUSTER": true,
		"DBSIZE": true, "LASTSAVE": true, "ROLE": true, "DEBUG": true,
	}
	return readCmds[cmd]
}

// IsWriteCommand 检查是否为写命令
func IsWriteCommand(cmd string) bool {
	writeCmds := map[string]bool{
		"SET": true, "SETEX": true, "SETNX": true, "PSETEX": true,
		"MSET": true, "MSETNX": true, "APPEND": true, "DECR": true,
		"DECRBY": true, "INCR": true, "INCRBY": true, "INCRBYFLOAT": true,
		"DEL": true, "UNLINK": true, "EXPIRE": true, "EXPIREAT": true,
		"PEXPIRE": true, "PEXPIREAT": true, "PERSIST": true, "RENAME": true,
		"RENAMENX": true, "LPUSH": true, "RPUSH": true, "LPOP": true,
		"RPOP": true, "LINSERT": true, "LREM": true, "LSET": true,
		"LTRIM": true, "SADD": true, "SREM": true, "SPOP": true,
		"SMOVE": true, "ZADD": true, "ZREM": true, "ZINCRBY": true,
		"ZREMRANGEBYRANK": true, "ZREMRANGEBYSCORE": true, "ZREMRANGEBYLEX": true,
		"ZUNIONSTORE": true, "ZINTERSTORE": true, "HSET": true, "HMSET": true,
		"HSETNX": true, "HDEL": true, "HINCRBY": true, "HINCRBYFLOAT": true,
		"PFADD": true, "PFMERGE": true, "FLUSHDB": true, "FLUSHALL": true,
	}
	return writeCmds[cmd]
}

// GetCommandType 获取命令类型
func GetCommandType(cmd string) string {
	cmdUpper := string(bytes.ToUpper([]byte(cmd)))

	if IsReadCommand(cmdUpper) {
		return "read"
	}
	if IsWriteCommand(cmdUpper) {
		return "write"
	}
	return "other"
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("redis", func() l7parser.Parser {
		return NewRedisParser()
	})
}
