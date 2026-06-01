// Package parsers MySQL 协议解析器
//
// MySQL 协议特点:
//   - 基于 TCP
//   - 请求/响应协议
//   - 支持 prepared statements
//   - 支持压缩和 SSL
//
// 实现要点:
//   - 解析 MySQL packet header (length + sequence)
//   - 识别常见命令 (QUERY, PREPARE, EXECUTE, etc.)
//   - 提取 SQL 语句

package parsers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidMySQL    = errors.New("invalid mysql packet")
	ErrIncompleteMySQL = errors.New("incomplete mysql packet")
)

// MySQL Command Types
const (
	MySQLComSleep            uint8 = 0
	MySQLComQuit             uint8 = 1
	MySQLComInitDB           uint8 = 2
	MySQLComQuery            uint8 = 3
	MySQLComFieldList        uint8 = 4
	MySQLComCreateDB         uint8 = 5
	MySQLComDropDB           uint8 = 6
	MySQLComRefresh          uint8 = 7
	MySQLComShutdown         uint8 = 8
	MySQLComStatistics       uint8 = 9
	MySQLComProcessInfo      uint8 = 10
	MySQLComConnect          uint8 = 11
	MySQLComProcessKill      uint8 = 12
	MySQLComDebug            uint8 = 13
	MySQLComPing             uint8 = 14
	MySQLComTime             uint8 = 15
	MySQLComDelayedInsert    uint8 = 16
	MySQLComChangeUser       uint8 = 17
	MySQLComBinlogDump       uint8 = 18
	MySQLComTableDump        uint8 = 19
	MySQLComConnectOut       uint8 = 20
	MySQLComRegisterSlave    uint8 = 21
	MySQLComStmtPrepare      uint8 = 22
	MySQLComStmtExecute      uint8 = 23
	MySQLComStmtSendLongData uint8 = 24
	MySQLComStmtClose        uint8 = 25
	MySQLComStmtReset        uint8 = 26
	MySQLComSetOption        uint8 = 27
	MySQLComStmtFetch        uint8 = 28
)

// MySQL Command Names
var MySQLCommandNames = map[uint8]string{
	MySQLComSleep:            "SLEEP",
	MySQLComQuit:             "QUIT",
	MySQLComInitDB:           "INIT_DB",
	MySQLComQuery:            "QUERY",
	MySQLComFieldList:        "FIELD_LIST",
	MySQLComCreateDB:         "CREATE_DB",
	MySQLComDropDB:           "DROP_DB",
	MySQLComRefresh:          "REFRESH",
	MySQLComShutdown:         "SHUTDOWN",
	MySQLComStatistics:       "STATISTICS",
	MySQLComProcessInfo:      "PROCESS_INFO",
	MySQLComConnect:          "CONNECT",
	MySQLComProcessKill:      "PROCESS_KILL",
	MySQLComDebug:            "DEBUG",
	MySQLComPing:             "PING",
	MySQLComTime:             "TIME",
	MySQLComDelayedInsert:    "DELAYED_INSERT",
	MySQLComChangeUser:       "CHANGE_USER",
	MySQLComBinlogDump:       "BINLOG_DUMP",
	MySQLComTableDump:        "TABLE_DUMP",
	MySQLComConnectOut:       "CONNECT_OUT",
	MySQLComRegisterSlave:    "REGISTER_SLAVE",
	MySQLComStmtPrepare:      "STMT_PREPARE",
	MySQLComStmtExecute:      "STMT_EXECUTE",
	MySQLComStmtSendLongData: "STMT_SEND_LONG_DATA",
	MySQLComStmtClose:        "STMT_CLOSE",
	MySQLComStmtReset:        "STMT_RESET",
	MySQLComSetOption:        "SET_OPTION",
	MySQLComStmtFetch:        "STMT_FETCH",
}

// GetMySQLCommandName 获取命令名称
func GetMySQLCommandName(cmd uint8) string {
	if name, ok := MySQLCommandNames[cmd]; ok {
		return name
	}
	return "UNKNOWN"
}

// MySQLPacketHeader MySQL 包头
type MySQLPacketHeader struct {
	Length    uint32
	Sequence  uint8
}

// MySQLCommandPacket MySQL 命令包
type MySQLCommandPacket struct {
	Header  MySQLPacketHeader
	Command uint8
	Arg     []byte
}

// MySQLParser MySQL 协议解析器
type MySQLParser struct {
	state *mysqlParseState
}

// mysqlParseState 解析状态
type mysqlParseState struct {
	buffer []byte
}

// NewMySQLParser 创建 MySQL 解析器
func NewMySQLParser() *MySQLParser {
	return &MySQLParser{
		state: &mysqlParseState{
			buffer: make([]byte, 0, 65536),
		},
	}
}

// Type 返回解析器类型
func (p *MySQLParser) Type() l7parser.ParserType {
	return l7parser.ParserTypeMySQL
}

// Name 返回解析器名称
func (p *MySQLParser) Name() string {
	return "mysql"
}

// Priority 返回解析优先级
func (p *MySQLParser) Priority() int {
	return 80
}

// Detect 协议检测
func (p *MySQLParser) Detect(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 5 {
		return false, 0
	}

	// 解析包头
	length := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
	if length < 1 || length > 0xFFFFFF {
		return false, 0
	}

	// 检查命令类型
	command := data[4]
	if command > 30 {
		return false, 0
	}

	// 检查端口
	if dstPort == 3306 || dstPort == 3307 {
		return true, 0.95
	}

	return true, 0.80
}

// Parse 解析数据包
func (p *MySQLParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	data := input.Packet.Data
	if len(data) == 0 {
		return nil, state, nil
	}

	// 获取或创建解析状态
	parseState, ok := state.(*mysqlParseState)
	if !ok {
		parseState = &mysqlParseState{
			buffer: make([]byte, 0, 65536),
		}
	}

	// 添加数据到缓冲区
	parseState.buffer = append(parseState.buffer, data...)

	result := &l7parser.ParseResult{
		ParserType: l7parser.ParserTypeMySQL,
		Headers:    make(map[string]string),
	}

	// 解析命令包
	for len(parseState.buffer) >= 5 {
		cmd, err := p.parseCommand(parseState)
		if err != nil {
			if err == ErrIncompleteMySQL {
				result.IsPartial = true
				result.NeedMore = true
				return result, parseState, nil
			}
			return nil, parseState, err
		}

		if cmd == nil {
			break
		}

		// 填充结果
		result.Direction = l7parser.DirRequest
		result.Headers["command"] = GetMySQLCommandName(cmd.Command)

		switch cmd.Command {
		case MySQLComQuery:
			// SQL 查询
			query := string(cmd.Arg)
			result.Headers["query"] = truncateString(query, 1024)
			result.Headers["operation"] = getSQLOperation(query)

		case MySQLComInitDB:
			// 选择数据库
			result.Headers["database"] = string(cmd.Arg)

		case MySQLComStmtPrepare:
			// Prepared statement
			result.Headers["prepare"] = truncateString(string(cmd.Arg), 1024)

		case MySQLComStmtExecute:
			// Execute prepared statement
			result.Headers["execute"] = "prepared_statement"
		}

		result.ReqSize = uint32(4 + cmd.Header.Length)
		result.IsComplete = true
	}

	return result, parseState, nil
}

// parseCommand 解析 MySQL 命令
func (p *MySQLParser) parseCommand(state *mysqlParseState) (*MySQLCommandPacket, error) {
	if len(state.buffer) < 5 {
		return nil, ErrIncompleteMySQL
	}

	// 解析包头 (3 bytes length + 1 byte sequence)
	length := uint32(state.buffer[0]) |
		uint32(state.buffer[1])<<8 |
		uint32(state.buffer[2])<<16
	sequence := state.buffer[3]

	if length < 1 || length > 0xFFFFFF {
		return nil, ErrInvalidMySQL
	}

	// 检查数据是否完整
	if uint32(len(state.buffer)) < 4+length {
		return nil, ErrIncompleteMySQL
	}

	// 解析命令
	command := state.buffer[4]
	var arg []byte
	if length > 1 {
		arg = make([]byte, length-1)
		copy(arg, state.buffer[5:4+length])
	}

	// 消费数据
	state.buffer = state.buffer[4+length:]

	return &MySQLCommandPacket{
		Header: MySQLPacketHeader{
			Length:   length,
			Sequence: sequence,
		},
		Command: command,
		Arg:     arg,
	}, nil
}

// ParseStreaming 流式解析
func (p *MySQLParser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *MySQLParser) Reset() {
	p.state = &mysqlParseState{
		buffer: make([]byte, 0, 65536),
	}
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getSQLOperation 获取 SQL 操作类型
func getSQLOperation(query string) string {
	query = strings.ToUpper(strings.TrimSpace(query))

	operations := []struct {
		prefix string
		op     string
	}{
		{"SELECT", "SELECT"},
		{"INSERT", "INSERT"},
		{"UPDATE", "UPDATE"},
		{"DELETE", "DELETE"},
		{"CREATE", "CREATE"},
		{"DROP", "DROP"},
		{"ALTER", "ALTER"},
		{"TRUNCATE", "TRUNCATE"},
		{"REPLACE", "REPLACE"},
		{"CALL", "CALL"},
		{"EXECUTE", "EXECUTE"},
		{"BEGIN", "BEGIN"},
		{"COMMIT", "COMMIT"},
		{"ROLLBACK", "ROLLBACK"},
		{"SET", "SET"},
		{"SHOW", "SHOW"},
		{"DESCRIBE", "DESCRIBE"},
		{"EXPLAIN", "EXPLAIN"},
		{"GRANT", "GRANT"},
		{"REVOKE", "REVOKE"},
	}

	for _, op := range operations {
		if strings.HasPrefix(query, op.prefix) {
			return op.op
		}
	}

	return "OTHER"
}

// IsReadOperation 检查是否为读操作
func IsReadOperation(operation string) bool {
	return operation == "SELECT" || operation == "SHOW" || operation == "DESCRIBE" || operation == "EXPLAIN"
}

// IsWriteOperation 检查是否为写操作
func IsWriteOperation(operation string) bool {
	writeOps := map[string]bool{
		"INSERT": true, "UPDATE": true, "DELETE": true,
		"CREATE": true, "DROP": true, "ALTER": true,
		"TRUNCATE": true, "REPLACE": true,
	}
	return writeOps[operation]
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("mysql", func() l7parser.Parser {
		return NewMySQLParser()
	})
}
