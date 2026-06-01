// Package dbobserver 提供数据库观测功能
// 本文件实现多数据库协议解析器
// 支持 MySQL、Oracle、PostgreSQL、达梦、GaussDB
package dbobserver

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// ==================== 协议解析器接口 ====================

// ProtocolParser 协议解析器接口
type ProtocolParser interface {
	// Parse 解析数据包，返回 SQL 事件
	Parse(data []byte, direction PacketDirection) (*SQLEvent, error)
	// GetDatabaseType 获取数据库类型
	GetDatabaseType() DatabaseType
	// IdentifyCommand 识别命令类型
	IdentifyCommand(data []byte) (string, error)
	// ExtractSQL 提取 SQL 语句
	ExtractSQL(data []byte) (string, error)
}

// PacketDirection 数据包方向
type PacketDirection int

const (
	DirectionClientToServer PacketDirection = iota // 客户端 -> 服务端
	DirectionServerToClient                        // 服务端 -> 客户端
)

// ==================== MySQL 协议解析器 ====================

// MySQLParser MySQL 协议解析器
type MySQLParser struct {
	// 状态机
	state      MySQLConnectionState
	stateMu    sync.RWMutex

	// 会话信息
	sessionID   string
	database    string
	user        string

	// 预处理语句
	preparedStmts map[uint32]*MySQLPreparedStatement

	// SQL 类型识别
	sqlTypeIdentifier *SQLTypeIdentifier
}

// MySQLConnectionState MySQL 连接状态
type MySQLConnectionState int

const (
	MySQLStateInit MySQLConnectionState = iota
	MySQLStateHandshake
	MySQLStateAuth
	MySQLStateReady
	MySQLStateQuery
	MySQLStatePrepare
	MySQLStateExecute
)

// MySQLPreparedStatement MySQL 预处理语句
type MySQLPreparedStatement struct {
	StatementID uint32
	SQL         string
	Params      int
}

// MySQLCommand MySQL 命令类型
type MySQLCommand byte

const (
	MySQLComSleep          MySQLCommand = 0x00
	MySQLComQuit           MySQLCommand = 0x01
	MySQLComInitDB         MySQLCommand = 0x02
	MySQLComQuery          MySQLCommand = 0x03
	MySQLComFieldList      MySQLCommand = 0x04
	MySQLComCreateDB       MySQLCommand = 0x05
	MySQLComDropDB         MySQLCommand = 0x06
	MySQLComRefresh        MySQLCommand = 0x07
	MySQLComShutdown       MySQLCommand = 0x08
	MySQLComStatistics     MySQLCommand = 0x09
	MySQLComProcessInfo    MySQLCommand = 0x0a
	MySQLComConnect        MySQLCommand = 0x0b
	MySQLComProcessKill    MySQLCommand = 0x0c
	MySQLComDebug          MySQLCommand = 0x0d
	MySQLComPing           MySQLCommand = 0x0e
	MySQLComTime           MySQLCommand = 0x0f
	MySQLComDelayedInsert  MySQLCommand = 0x10
	MySQLComChangeUser     MySQLCommand = 0x11
	MySQLComBinlogDump     MySQLCommand = 0x12
	MySQLComTableDump      MySQLCommand = 0x13
	MySQLComConnectOut     MySQLCommand = 0x14
	MySQLComRegisterSlave  MySQLCommand = 0x15
	MySQLComStmtPrepare    MySQLCommand = 0x16
	MySQLComStmtExecute    MySQLCommand = 0x17
	MySQLComStmtSendLongData MySQLCommand = 0x18
	MySQLComStmtClose      MySQLCommand = 0x19
	MySQLComStmtReset      MySQLCommand = 0x1a
	MySQLComSetOption      MySQLCommand = 0x1b
	MySQLComStmtFetch      MySQLCommand = 0x1c
)

// NewMySQLParser 创建 MySQL 协议解析器
func NewMySQLParser() *MySQLParser {
	return &MySQLParser{
		state:             MySQLStateInit,
		preparedStmts:     make(map[uint32]*MySQLPreparedStatement),
		sqlTypeIdentifier: NewSQLTypeIdentifier(),
	}
}

// Parse 解析 MySQL 数据包
func (p *MySQLParser) Parse(data []byte, direction PacketDirection) (*SQLEvent, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("数据包太短")
	}

	// 解析 MySQL 包头
	// 前 3 字节是 payload 长度，第 4 字节是 sequence ID
	payloadLen := int(data[0]) | int(data[1])<<8 | int(data[2])<<16
	sequenceID := data[3]
	payload := data[4:]

	if len(payload) < payloadLen {
		return nil, fmt.Errorf("payload 长度不匹配")
	}

	// 只处理客户端到服务端的请求
	if direction == DirectionClientToServer {
		return p.parseClientPacket(payload, sequenceID)
	}

	return nil, nil
}

// parseClientPacket 解析客户端数据包
func (p *MySQLParser) parseClientPacket(payload []byte, sequenceID byte) (*SQLEvent, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	cmd := MySQLCommand(payload[0])

	switch cmd {
	case MySQLComQuery:
		// COM_QUERY 命令
		sql := string(payload[1:])
		return p.createSQLEvent(sql), nil

	case MySQLComInitDB:
		// COM_INIT_DB 命令
		dbName := string(payload[1:])
		p.database = dbName
		return nil, nil

	case MySQLComStmtPrepare:
		// COM_STMT_PREPARE 命令
		sql := string(payload[1:])
		// 预处理语句会在响应中获取 statement ID
		return p.createSQLEvent(sql), nil

	case MySQLComStmtExecute:
		// COM_STMT_EXECUTE 命令
		if len(payload) < 5 {
			return nil, nil
		}
		stmtID := binary.LittleEndian.Uint32(payload[1:5])
		if stmt, ok := p.preparedStmts[stmtID]; ok {
			return p.createSQLEvent(stmt.SQL), nil
		}

	case MySQLComStmtClose:
		// COM_STMT_CLOSE 命令
		if len(payload) >= 5 {
			stmtID := binary.LittleEndian.Uint32(payload[1:5])
			delete(p.preparedStmts, stmtID)
		}
		return nil, nil
	}

	return nil, nil
}

// createSQLEvent 创建 SQL 事件
func (p *MySQLParser) createSQLEvent(sql string) *SQLEvent {
	return &SQLEvent{
		DatabaseType: DatabaseTypeMySQL,
		DatabaseName: p.database,
		User:         p.user,
		SQLText:      sql,
		SQLType:      p.sqlTypeIdentifier.Identify(sql),
	}
}

// GetDatabaseType 获取数据库类型
func (p *MySQLParser) GetDatabaseType() DatabaseType {
	return DatabaseTypeMySQL
}

// IdentifyCommand 识别命令类型
func (p *MySQLParser) IdentifyCommand(data []byte) (string, error) {
	if len(data) < 5 {
		return "", fmt.Errorf("数据包太短")
	}

	cmd := MySQLCommand(data[4])
	switch cmd {
	case MySQLComQuery:
		return "QUERY", nil
	case MySQLComInitDB:
		return "INIT_DB", nil
	case MySQLComStmtPrepare:
		return "STMT_PREPARE", nil
	case MySQLComStmtExecute:
		return "STMT_EXECUTE", nil
	case MySQLComPing:
		return "PING", nil
	case MySQLComQuit:
		return "QUIT", nil
	default:
		return fmt.Sprintf("UNKNOWN(%d)", cmd), nil
	}
}

// ExtractSQL 提取 SQL 语句
func (p *MySQLParser) ExtractSQL(data []byte) (string, error) {
	if len(data) < 5 {
		return "", fmt.Errorf("数据包太短")
	}

	cmd := MySQLCommand(data[4])
	if cmd == MySQLComQuery {
		return string(data[5:]), nil
	}

	return "", nil
}

// ==================== PostgreSQL 协议解析器 ====================

// PostgreSQLParser PostgreSQL 协议解析器
type PostgreSQLParser struct {
	// 状态机
	state      PostgreSQLConnectionState
	stateMu    sync.RWMutex

	// 会话信息
	sessionID   string
	database    string
	user        string

	// 预处理语句
	preparedStmts map[string]*PostgreSQLPreparedStatement
	portalStmts   map[string]string // portal -> statement name

	// SQL 类型识别
	sqlTypeIdentifier *SQLTypeIdentifier
}

// PostgreSQLConnectionState PostgreSQL 连接状态
type PostgreSQLConnectionState int

const (
	PostgreSQLStateInit PostgreSQLConnectionState = iota
	PostgreSQLStateStartup
	PostgreSQLStateAuth
	PostgreSQLStateReady
	PostgreSQLStateQuery
)

// PostgreSQLPreparedStatement PostgreSQL 预处理语句
type PostgreSQLPreparedStatement struct {
	Name       string
	SQL        string
	ParamTypes []uint32
}

// PostgreSQLMessageType PostgreSQL 消息类型
type PostgreSQLMessageType byte

const (
	PostgreSQLMsgQuery         PostgreSQLMessageType = 'Q'
	PostgreSQLMsgParse         PostgreSQLMessageType = 'P'
	PostgreSQLMsgBind          PostgreSQLMessageType = 'B'
	PostgreSQLMsgDescribe      PostgreSQLMessageType = 'D'
	PostgreSQLMsgExecute       PostgreSQLMessageType = 'E'
	PostgreSQLMsgSync          PostgreSQLMessageType = 'S'
	PostgreSQLMsgClose         PostgreSQLMessageType = 'C'
	PostgreSQLMsgTerminate     PostgreSQLMessageType = 'X'
	PostgreSQLMsgCopyData      PostgreSQLMessageType = 'd'
	PostgreSQLMsgCopyDone      PostgreSQLMessageType = 'c'
	PostgreSQLMsgCopyFail      PostgreSQLMessageType = 'f'
)

// NewPostgreSQLParser 创建 PostgreSQL 协议解析器
func NewPostgreSQLParser() *PostgreSQLParser {
	return &PostgreSQLParser{
		state:             PostgreSQLStateInit,
		preparedStmts:     make(map[string]*PostgreSQLPreparedStatement),
		portalStmts:       make(map[string]string),
		sqlTypeIdentifier: NewSQLTypeIdentifier(),
	}
}

// Parse 解析 PostgreSQL 数据包
func (p *PostgreSQLParser) Parse(data []byte, direction PacketDirection) (*SQLEvent, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("数据包太短")
	}

	// PostgreSQL 消息格式：1字节类型 + 4字节长度 + payload
	msgType := PostgreSQLMessageType(data[0])
	length := int(binary.BigEndian.Uint32(data[1:5]))
	payload := data[5:]

	if len(payload) < length-4 {
		return nil, fmt.Errorf("payload 长度不匹配")
	}

	// 只处理客户端到服务端的请求
	if direction == DirectionClientToServer {
		return p.parseClientMessage(msgType, payload)
	}

	return nil, nil
}

// parseClientMessage 解析客户端消息
func (p *PostgreSQLParser) parseClientMessage(msgType PostgreSQLMessageType, payload []byte) (*SQLEvent, error) {
	switch msgType {
	case PostgreSQLMsgQuery:
		// 简单查询
		sql := p.extractCString(payload)
		return p.createSQLEvent(sql), nil

	case PostgreSQLMsgParse:
		// 解析预处理语句
		return p.parseParseMessage(payload)

	case PostgreSQLMsgBind:
		// 绑定参数
		return p.parseBindMessage(payload)

	case PostgreSQLMsgExecute:
		// 执行语句
		return p.parseExecuteMessage(payload)

	case PostgreSQLMsgDescribe:
		// 描述语句
		return nil, nil

	case PostgreSQLMsgClose:
		// 关闭语句
		return p.parseCloseMessage(payload)

	case PostgreSQLMsgTerminate:
		// 终止连接
		return nil, nil
	}

	return nil, nil
}

// parseParseMessage 解析 Parse 消息
func (p *PostgreSQLParser) parseParseMessage(payload []byte) (*SQLEvent, error) {
	// Parse 消息格式：
	// statement name (null-terminated)
	// query (null-terminated)
	// number of parameters (2 bytes)
	// parameter types (4 bytes each)

	// 提取 statement name
	name, rest := p.extractCStringWithRest(payload)
	// 提取 SQL
	sql, _ := p.extractCStringWithRest(rest)

	// 存储预处理语句
	p.preparedStmts[name] = &PostgreSQLPreparedStatement{
		Name: name,
		SQL:  sql,
	}

	return p.createSQLEvent(sql), nil
}

// parseBindMessage 解析 Bind 消息
func (p *PostgreSQLParser) parseBindMessage(payload []byte) (*SQLEvent, error) {
	// Bind 消息格式：
	// portal name (null-terminated)
	// statement name (null-terminated)

	portal, rest := p.extractCStringWithRest(payload)
	stmtName, _ := p.extractCStringWithRest(rest)

	// 关联 portal 和 statement
	p.portalStmts[portal] = stmtName

	return nil, nil
}

// parseExecuteMessage 解析 Execute 消息
func (p *PostgreSQLParser) parseExecuteMessage(payload []byte) (*SQLEvent, error) {
	// Execute 消息格式：
	// portal name (null-terminated)
	// max rows (4 bytes)

	portal, _ := p.extractCStringWithRest(payload)
	stmtName := p.portalStmts[portal]

	if stmt, ok := p.preparedStmts[stmtName]; ok {
		return p.createSQLEvent(stmt.SQL), nil
	}

	return nil, nil
}

// parseCloseMessage 解析 Close 消息
func (p *PostgreSQLParser) parseCloseMessage(payload []byte) (*SQLEvent, error) {
	if len(payload) < 2 {
		return nil, nil
	}

	closeType := payload[0] // 'S' for statement, 'P' for portal
	name := p.extractCString(payload[1:])

	if closeType == 'S' {
		delete(p.preparedStmts, name)
	} else if closeType == 'P' {
		delete(p.portalStmts, name)
	}

	return nil, nil
}

// createSQLEvent 创建 SQL 事件
func (p *PostgreSQLParser) createSQLEvent(sql string) *SQLEvent {
	return &SQLEvent{
		DatabaseType: DatabaseTypePostgreSQL,
		DatabaseName: p.database,
		User:         p.user,
		SQLText:      sql,
		SQLType:      p.sqlTypeIdentifier.Identify(sql),
	}
}

// extractCString 提取 C 风格字符串
func (p *PostgreSQLParser) extractCString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

// extractCStringWithRest 提取 C 风格字符串并返回剩余部分
func (p *PostgreSQLParser) extractCStringWithRest(data []byte) (string, []byte) {
	for i, b := range data {
		if b == 0 {
			return string(data[:i]), data[i+1:]
		}
	}
	return string(data), nil
}

// GetDatabaseType 获取数据库类型
func (p *PostgreSQLParser) GetDatabaseType() DatabaseType {
	return DatabaseTypePostgreSQL
}

// IdentifyCommand 识别命令类型
func (p *PostgreSQLParser) IdentifyCommand(data []byte) (string, error) {
	if len(data) < 1 {
		return "", fmt.Errorf("数据包太短")
	}

	msgType := PostgreSQLMessageType(data[0])
	switch msgType {
	case PostgreSQLMsgQuery:
		return "QUERY", nil
	case PostgreSQLMsgParse:
		return "PARSE", nil
	case PostgreSQLMsgBind:
		return "BIND", nil
	case PostgreSQLMsgExecute:
		return "EXECUTE", nil
	case PostgreSQLMsgDescribe:
		return "DESCRIBE", nil
	case PostgreSQLMsgClose:
		return "CLOSE", nil
	case PostgreSQLMsgTerminate:
		return "TERMINATE", nil
	default:
		return fmt.Sprintf("UNKNOWN(%c)", msgType), nil
	}
}

// ExtractSQL 提取 SQL 语句
func (p *PostgreSQLParser) ExtractSQL(data []byte) (string, error) {
	if len(data) < 5 {
		return "", fmt.Errorf("数据包太短")
	}

	msgType := PostgreSQLMessageType(data[0])
	if msgType == PostgreSQLMsgQuery {
		return p.extractCString(data[5:]), nil
	}

	return "", nil
}

// ==================== Oracle 协议解析器 ====================

// OracleParser Oracle 协议解析器
type OracleParser struct {
	// 状态机
	state      OracleConnectionState
	stateMu    sync.RWMutex

	// 会话信息
	sessionID   string
	serviceName string
	user        string

	// SQL 类型识别
	sqlTypeIdentifier *SQLTypeIdentifier

	// Oracle 特有的模式
	sqlPatterns []*regexp.Regexp
}

// OracleConnectionState Oracle 连接状态
type OracleConnectionState int

const (
	OracleStateInit OracleConnectionState = iota
	OracleStateConnect
	OracleStateAuth
	OracleStateReady
	OracleStateExecute
)

// NewOracleParser 创建 Oracle 协议解析器
func NewOracleParser() *OracleParser {
	return &OracleParser{
		state:             OracleStateInit,
		sqlTypeIdentifier: NewSQLTypeIdentifier(),
		sqlPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)SELECT\s+.*\s+FROM`),
			regexp.MustCompile(`(?i)INSERT\s+INTO`),
			regexp.MustCompile(`(?i)UPDATE\s+.*\s+SET`),
			regexp.MustCompile(`(?i)DELETE\s+FROM`),
		},
	}
}

// Parse 解析 Oracle 数据包
func (p *OracleParser) Parse(data []byte, direction PacketDirection) (*SQLEvent, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("数据包太短")
	}

	// Oracle TNS 包格式：
	// 2 字节长度
	// 2 字节包校验和
	// 1 字节类型
	// 1 字节保留
	// payload

	// 只处理客户端到服务端的请求
	if direction == DirectionClientToServer {
		return p.parseClientPacket(data)
	}

	return nil, nil
}

// parseClientPacket 解析客户端数据包
func (p *OracleParser) parseClientPacket(data []byte) (*SQLEvent, error) {
	// Oracle 协议比较复杂，这里简化处理
	// 主要关注 SQL 执行相关的包

	// 尝试从 payload 中提取 SQL
	sql := p.tryExtractSQL(data)
	if sql != "" {
		return p.createSQLEvent(sql), nil
	}

	return nil, nil
}

// tryExtractSQL 尝试提取 SQL
func (p *OracleParser) tryExtractSQL(data []byte) string {
	// Oracle SQL 通常以特定格式嵌入在包中
	// 这里使用简化的模式匹配

	dataStr := string(data)

	// 查找 SQL 关键字
	for _, pattern := range p.sqlPatterns {
		if match := pattern.FindString(dataStr); match != "" {
			// 尝试提取完整的 SQL 语句
			return p.extractFullSQL(dataStr, match)
		}
	}

	return ""
}

// extractFullSQL 提取完整 SQL
func (p *OracleParser) extractFullSQL(data, match string) string {
	// 简化实现：从匹配位置开始，找到语句结束符
	startIdx := strings.Index(data, match)
	if startIdx == -1 {
		return match
	}

	// 查找语句结束（分号或 null 字节）
	endIdx := startIdx
	for i := startIdx; i < len(data); i++ {
		if data[i] == ';' || data[i] == 0 {
			endIdx = i
			break
		}
	}

	if endIdx > startIdx {
		return strings.TrimSpace(data[startIdx:endIdx])
	}

	return match
}

// createSQLEvent 创建 SQL 事件
func (p *OracleParser) createSQLEvent(sql string) *SQLEvent {
	return &SQLEvent{
		DatabaseType: DatabaseTypeOracle,
		DatabaseName: p.serviceName,
		User:         p.user,
		SQLText:      sql,
		SQLType:      p.sqlTypeIdentifier.Identify(sql),
	}
}

// GetDatabaseType 获取数据库类型
func (p *OracleParser) GetDatabaseType() DatabaseType {
	return DatabaseTypeOracle
}

// IdentifyCommand 识别命令类型
func (p *OracleParser) IdentifyCommand(data []byte) (string, error) {
	if len(data) < 8 {
		return "", fmt.Errorf("数据包太短")
	}

	// Oracle 包类型在第 4 字节
	packetType := data[4]

	switch packetType {
	case 0x01:
		return "CONNECT", nil
	case 0x02:
		return "ACCEPT", nil
	case 0x03:
		return "ACK", nil
	case 0x04:
		return "REFUSE", nil
	case 0x06:
		return "DATA", nil
	case 0x0b:
		return "EXECUTE", nil
	case 0x0e:
		return "FETCH", nil
	default:
		return fmt.Sprintf("UNKNOWN(%d)", packetType), nil
	}
}

// ExtractSQL 提取 SQL 语句
func (p *OracleParser) ExtractSQL(data []byte) (string, error) {
	return p.tryExtractSQL(data), nil
}

// ==================== 达梦协议解析器 ====================

// DaMengParser 达梦协议解析器
type DaMengParser struct {
	// 状态机
	state      DaMengConnectionState
	stateMu    sync.RWMutex

	// 会话信息
	sessionID   string
	database    string
	user        string

	// SQL 类型识别
	sqlTypeIdentifier *SQLTypeIdentifier
}

// DaMengConnectionState 达梦连接状态
type DaMengConnectionState int

const (
	DaMengStateInit DaMengConnectionState = iota
	DaMengStateHandshake
	DaMengStateAuth
	DaMengStateReady
	DaMengStateExecute
)

// NewDaMengParser 创建达梦协议解析器
func NewDaMengParser() *DaMengParser {
	return &DaMengParser{
		state:             DaMengStateInit,
		sqlTypeIdentifier: NewSQLTypeIdentifier(),
	}
}

// Parse 解析达梦数据包
func (p *DaMengParser) Parse(data []byte, direction PacketDirection) (*SQLEvent, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("数据包太短")
	}

	// 达梦协议格式（类似 Oracle 但有差异）：
	// 4 字节长度
	// 4 字节命令类型
	// 4 字节序列号
	// payload

	// 只处理客户端到服务端的请求
	if direction == DirectionClientToServer {
		return p.parseClientPacket(data)
	}

	return nil, nil
}

// parseClientPacket 解析客户端数据包
func (p *DaMengParser) parseClientPacket(data []byte) (*SQLEvent, error) {
	// 达梦协议解析
	// 这里简化处理，主要提取 SQL 语句

	sql := p.tryExtractSQL(data)
	if sql != "" {
		return p.createSQLEvent(sql), nil
	}

	return nil, nil
}

// tryExtractSQL 尝试提取 SQL
func (p *DaMengParser) tryExtractSQL(data []byte) string {
	// 达梦 SQL 语句通常在 payload 部分
	// 使用通用的 SQL 模式匹配

	dataStr := string(data)

	// 查找 SQL 关键字
	upperStr := strings.ToUpper(dataStr)
	keywords := []string{"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "CREATE ", "ALTER ", "DROP "}

	for _, keyword := range keywords {
		if idx := strings.Index(upperStr, keyword); idx != -1 {
			// 提取从关键字开始的内容
			rest := dataStr[idx:]
			// 找到语句结束
			for i, ch := range rest {
				if ch == 0 || ch == ';' {
					return strings.TrimSpace(rest[:i])
				}
			}
			// 如果没有结束符，取前 1024 字节
			if len(rest) > 1024 {
				return strings.TrimSpace(rest[:1024])
			}
			return strings.TrimSpace(rest)
		}
	}

	return ""
}

// createSQLEvent 创建 SQL 事件
func (p *DaMengParser) createSQLEvent(sql string) *SQLEvent {
	return &SQLEvent{
		DatabaseType: DatabaseTypeDaMeng,
		DatabaseName: p.database,
		User:         p.user,
		SQLText:      sql,
		SQLType:      p.sqlTypeIdentifier.Identify(sql),
	}
}

// GetDatabaseType 获取数据库类型
func (p *DaMengParser) GetDatabaseType() DatabaseType {
	return DatabaseTypeDaMeng
}

// IdentifyCommand 识别命令类型
func (p *DaMengParser) IdentifyCommand(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("数据包太短")
	}

	// 达梦命令类型在第 5-8 字节
	cmdType := binary.LittleEndian.Uint32(data[4:8])

	switch cmdType {
	case 0x01:
		return "CONNECT", nil
	case 0x02:
		return "QUERY", nil
	case 0x03:
		return "FETCH", nil
	case 0x04:
		return "EXECUTE", nil
	case 0x05:
		return "PREPARE", nil
	default:
		return fmt.Sprintf("UNKNOWN(%d)", cmdType), nil
	}
}

// ExtractSQL 提取 SQL 语句
func (p *DaMengParser) ExtractSQL(data []byte) (string, error) {
	return p.tryExtractSQL(data), nil
}

// ==================== GaussDB 协议解析器 ====================

// GaussDBParser GaussDB 协议解析器
type GaussDBParser struct {
	// GaussDB 协议与 PostgreSQL 高度兼容
	// 使用 PostgreSQL 协议作为基础
	pgParser *PostgreSQLParser

	// 状态机
	state      GaussDBConnectionState
	stateMu    sync.RWMutex

	// 会话信息
	sessionID   string
	database    string
	user        string

	// SQL 类型识别
	sqlTypeIdentifier *SQLTypeIdentifier
}

// GaussDBConnectionState GaussDB 连接状态
type GaussDBConnectionState int

const (
	GaussDBStateInit GaussDBConnectionState = iota
	GaussDBStateStartup
	GaussDBStateAuth
	GaussDBStateReady
	GaussDBStateQuery
)

// NewGaussDBParser 创建 GaussDB 协议解析器
func NewGaussDBParser() *GaussDBParser {
	return &GaussDBParser{
		pgParser:          NewPostgreSQLParser(),
		state:             GaussDBStateInit,
		sqlTypeIdentifier: NewSQLTypeIdentifier(),
	}
}

// Parse 解析 GaussDB 数据包
func (p *GaussDBParser) Parse(data []byte, direction PacketDirection) (*SQLEvent, error) {
	// GaussDB 与 PostgreSQL 协议兼容
	// 首先尝试 PostgreSQL 协议解析
	event, err := p.pgParser.Parse(data, direction)
	if err == nil && event != nil {
		// 修改数据库类型
		event.DatabaseType = DatabaseTypeGaussDB
		return event, nil
	}

	// 如果 PostgreSQL 解析失败，尝试 GaussDB 特有格式
	if len(data) < 5 {
		return nil, fmt.Errorf("数据包太短")
	}

	// 只处理客户端到服务端的请求
	if direction == DirectionClientToServer {
		return p.parseClientPacket(data)
	}

	return nil, nil
}

// parseClientPacket 解析客户端数据包
func (p *GaussDBParser) parseClientPacket(data []byte) (*SQLEvent, error) {
	// GaussDB 特有的扩展协议处理
	// 这里简化处理

	sql := p.tryExtractSQL(data)
	if sql != "" {
		return p.createSQLEvent(sql), nil
	}

	return nil, nil
}

// tryExtractSQL 尝试提取 SQL
func (p *GaussDBParser) tryExtractSQL(data []byte) string {
	dataStr := string(data)

	// 查找 SQL 关键字
	upperStr := strings.ToUpper(dataStr)
	keywords := []string{"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "CREATE ", "ALTER ", "DROP "}

	for _, keyword := range keywords {
		if idx := strings.Index(upperStr, keyword); idx != -1 {
			rest := dataStr[idx:]
			for i, ch := range rest {
				if ch == 0 || ch == ';' {
					return strings.TrimSpace(rest[:i])
				}
			}
			if len(rest) > 1024 {
				return strings.TrimSpace(rest[:1024])
			}
			return strings.TrimSpace(rest)
		}
	}

	return ""
}

// createSQLEvent 创建 SQL 事件
func (p *GaussDBParser) createSQLEvent(sql string) *SQLEvent {
	return &SQLEvent{
		DatabaseType: DatabaseTypeGaussDB,
		DatabaseName: p.database,
		User:         p.user,
		SQLText:      sql,
		SQLType:      p.sqlTypeIdentifier.Identify(sql),
	}
}

// GetDatabaseType 获取数据库类型
func (p *GaussDBParser) GetDatabaseType() DatabaseType {
	return DatabaseTypeGaussDB
}

// IdentifyCommand 识别命令类型
func (p *GaussDBParser) IdentifyCommand(data []byte) (string, error) {
	// 优先使用 PostgreSQL 协议识别
	return p.pgParser.IdentifyCommand(data)
}

// ExtractSQL 提取 SQL 语句
func (p *GaussDBParser) ExtractSQL(data []byte) (string, error) {
	// 优先使用 PostgreSQL 协议提取
	sql, err := p.pgParser.ExtractSQL(data)
	if err == nil && sql != "" {
		return sql, nil
	}

	return p.tryExtractSQL(data), nil
}

// ==================== 协议解析器工厂 ====================

// NewProtocolParser 创建协议解析器
func NewProtocolParser(dbType DatabaseType) ProtocolParser {
	switch dbType {
	case DatabaseTypeMySQL:
		return NewMySQLParser()
	case DatabaseTypeOracle:
		return NewOracleParser()
	case DatabaseTypePostgreSQL:
		return NewPostgreSQLParser()
	case DatabaseTypeDaMeng:
		return NewDaMengParser()
	case DatabaseTypeGaussDB:
		return NewGaussDBParser()
	default:
		return nil
	}
}

// ==================== 协议识别器 ====================

// ProtocolIdentifier 协议识别器
type ProtocolIdentifier struct {
	// 各数据库协议特征
	mysqlSignature      []byte
	oracleSignature     []byte
	postgresqlSignature []byte
	damengSignature     []byte
	gaussdbSignature    []byte
}

// NewProtocolIdentifier 创建协议识别器
func NewProtocolIdentifier() *ProtocolIdentifier {
	return &ProtocolIdentifier{
		mysqlSignature:      []byte{0x0a}, // MySQL 协议版本
		oracleSignature:     []byte{0x00, 0x00, 0x01, 0x00}, // Oracle TNS
		postgresqlSignature: []byte{0x00, 0x03, 0x00, 0x00}, // PostgreSQL 3.0
		damengSignature:     []byte{0x00, 0x00, 0x00, 0x01}, // 达梦协议
		gaussdbSignature:    []byte{0x00, 0x03, 0x00, 0x00}, // GaussDB 兼容 PostgreSQL
	}
}

// Identify 识别数据库协议类型
func (i *ProtocolIdentifier) Identify(data []byte, port int) DatabaseType {
	// 首先根据端口判断
	switch port {
	case 3306:
		return DatabaseTypeMySQL
	case 1521:
		return DatabaseTypeOracle
	case 5432:
		return DatabaseTypePostgreSQL
	case 5236:
		return DatabaseTypeDaMeng
	case 8000, 6432:
		return DatabaseTypeGaussDB
	}

	// 根据协议特征判断
	if len(data) < 4 {
		return DatabaseTypeUnknown
	}

	// MySQL 协议特征
	if data[0] == 0x0a && len(data) > 10 {
		return DatabaseTypeMySQL
	}

	// PostgreSQL 协议特征
	if len(data) >= 8 {
		length := int(binary.BigEndian.Uint32(data[0:4]))
		if length == 8 && data[4] == 0x00 && data[5] == 0x03 {
			return DatabaseTypePostgreSQL
		}
	}

	// Oracle TNS 协议特征
	if len(data) >= 4 {
		length := int(binary.BigEndian.Uint16(data[0:2]))
		if length > 0 && length < 0xFFFF && data[4] == 0x01 {
			return DatabaseTypeOracle
		}
	}

	// 达梦协议特征
	if len(data) >= 12 {
		length := binary.LittleEndian.Uint32(data[0:4])
		if length > 0 && length < 0xFFFFF {
			return DatabaseTypeDaMeng
		}
	}

	return DatabaseTypeUnknown
}

// ==================== 数据包捕获器 ====================

// PacketCapture 数据包捕获器
type PacketCapture struct {
	parsers       map[DatabaseType]ProtocolParser
	identifier    *ProtocolIdentifier
	eventHandler  func(*SQLEvent)
	mu            sync.RWMutex
}

// NewPacketCapture 创建数据包捕获器
func NewPacketCapture() *PacketCapture {
	capture := &PacketCapture{
		parsers:    make(map[DatabaseType]ProtocolParser),
		identifier: NewProtocolIdentifier(),
	}

	// 初始化所有解析器
	for _, dbType := range []DatabaseType{
		DatabaseTypeMySQL,
		DatabaseTypeOracle,
		DatabaseTypePostgreSQL,
		DatabaseTypeDaMeng,
		DatabaseTypeGaussDB,
	} {
		capture.parsers[dbType] = NewProtocolParser(dbType)
	}

	return capture
}

// SetEventHandler 设置事件处理器
func (c *PacketCapture) SetEventHandler(handler func(*SQLEvent)) {
	c.eventHandler = handler
}

// ProcessPacket 处理数据包
func (c *PacketCapture) ProcessPacket(data []byte, direction PacketDirection, port int) error {
	// 识别协议类型
	dbType := c.identifier.Identify(data, port)
	if dbType == DatabaseTypeUnknown {
		return fmt.Errorf("无法识别数据库协议")
	}

	// 获取解析器
	c.mu.RLock()
	parser, ok := c.parsers[dbType]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("不支持的数据库类型: %s", dbType)
	}

	// 解析数据包
	event, err := parser.Parse(data, direction)
	if err != nil {
		return err
	}

	// 处理事件
	if event != nil && c.eventHandler != nil {
		c.eventHandler(event)
	}

	return nil
}

// GetParser 获取指定类型的解析器
func (c *PacketCapture) GetParser(dbType DatabaseType) ProtocolParser {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.parsers[dbType]
}
