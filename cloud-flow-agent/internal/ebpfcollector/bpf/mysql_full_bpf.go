// mysql_full_bpf.go - Go绑定用于加载MySQL全字段解析eBPF程序
//go:build linux
// +build linux

package bpf

import (
	"bytes"
	"embed"
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

//go:embed mysql_full.bpf.o
var mysqlFullBpfFS embed.FS

// MySQLFullObjects 包含所有MySQL全字段解析eBPF对象
type MySQLFullObjects struct {
	// Programs
	TraceMySQLRecvmsg         *ebpf.Program `ebpf:"trace_mysql_recvmsg"`
	TraceMySQLSendmsg         *ebpf.Program `ebpf:"trace_mysql_sendmsg"`
	TraceMySQLRecvmsgResponse *ebpf.Program `ebpf:"trace_mysql_recvmsg_response"`
	TraceMySQLTcpClose        *ebpf.Program `ebpf:"trace_mysql_tcp_close"`

	// Maps
	MySQLConnectionsMap *ebpf.Map `ebpf:"mysql_connections"`
	MySQLEventsMap      *ebpf.Map `ebpf:"mysql_events"`
	MySQLCmdStatsMap    *ebpf.Map `ebpf:"mysql_cmd_stats"`
	MySQLErrorStatsMap  *ebpf.Map `ebpf:"mysql_error_stats"`
}

// MySQL命令类型常量
const (
	MYSQL_COM_SLEEP              uint8 = 0
	MYSQL_COM_QUIT               uint8 = 1
	MYSQL_COM_INIT_DB            uint8 = 2
	MYSQL_COM_QUERY              uint8 = 3
	MYSQL_COM_FIELD_LIST         uint8 = 4
	MYSQL_COM_CREATE_DB          uint8 = 5
	MYSQL_COM_DROP_DB            uint8 = 6
	MYSQL_COM_REFRESH            uint8 = 7
	MYSQL_COM_SHUTDOWN           uint8 = 8
	MYSQL_COM_STATISTICS         uint8 = 9
	MYSQL_COM_PROCESS_INFO       uint8 = 10
	MYSQL_COM_CONNECT            uint8 = 11
	MYSQL_COM_PROCESS_KILL       uint8 = 12
	MYSQL_COM_DEBUG              uint8 = 13
	MYSQL_COM_PING               uint8 = 14
	MYSQL_COM_TIME               uint8 = 15
	MYSQL_COM_DELAYED_INSERT     uint8 = 16
	MYSQL_COM_CHANGE_USER        uint8 = 17
	MYSQL_COM_BINLOG_DUMP        uint8 = 18
	MYSQL_COM_TABLE_DUMP         uint8 = 19
	MYSQL_COM_CONNECT_OUT        uint8 = 20
	MYSQL_COM_REGISTER_SLAVE     uint8 = 21
	MYSQL_COM_STMT_PREPARE       uint8 = 22
	MYSQL_COM_STMT_EXECUTE       uint8 = 23
	MYSQL_COM_STMT_SEND_LONG_DATA uint8 = 24
	MYSQL_COM_STMT_CLOSE         uint8 = 25
	MYSQL_COM_STMT_RESET         uint8 = 26
	MYSQL_COM_SET_OPTION         uint8 = 27
	MYSQL_COM_STMT_FETCH         uint8 = 28
)

// MySQL包类型常量
const (
	MYSQL_PACKET_OK   uint8 = 0x00
	MYSQL_PACKET_ERR  uint8 = 0xFF
	MYSQL_PACKET_EOF  uint8 = 0xFE
	MYSQL_PACKET_AUTH uint8 = 0x01
)

// MySQL错误码常量
const (
	MYSQL_ERR_ACCESS_DENIED uint16 = 1045
	MYSQL_ERR_BAD_DB        uint16 = 1049
	MYSQL_ERR_TABLE_EXISTS  uint16 = 1050
	MYSQL_ERR_BAD_TABLE     uint16 = 1051
	MYSQL_ERR_NO_SUCH_TABLE uint16 = 1146
	MYSQL_ERR_PARSE_ERROR   uint16 = 1064
	MYSQL_ERR_CONN_LOST     uint16 = 2013
)

// MySQLConnKey MySQL连接标识
type MySQLConnKey struct {
	Saddr uint32
	Daddr uint32
	Sport uint16
	Dport uint16
	Pid   uint32
	Netns uint32
}

// MySQLHandshake MySQL握手信息
type MySQLHandshake struct {
	ProtocolVersion    uint8
	ServerVersion      [32]byte
	ConnectionId       uint32
	AuthPluginData     [21]byte
	CapabilityFlags    uint32
	CharacterSet       uint8
	StatusFlags        uint16
	AuthPluginDataLen  uint16
}

// MySQLAuth MySQL认证信息
type MySQLAuth struct {
	CapabilityFlags   uint32
	MaxPacketSize     uint32
	CharacterSet      uint8
	Username          [32]byte
	AuthResponse      [256]byte
	AuthResponseLen   uint8
	Database          [64]byte
	AuthPluginName    [32]byte
}

// MySQLCommand MySQL命令信息
type MySQLCommand struct {
	TimestampNs    uint64
	Command        uint8
	ArgLen         uint32
	Argument       [1024]byte
	ArgumentLen    uint16

	// 解析的SQL信息
	Tables         [4][64]byte
	TableCount     uint8
	IsSelect       uint8
	IsInsert       uint8
	IsUpdate       uint8
	IsDelete       uint8
	IsDDL          uint8
	IsDCL          uint8
	IsTransaction  uint8
	Padding        uint8
}

// MySQLResponse MySQL响应信息
type MySQLResponse struct {
	TimestampNs      uint64
	LatencyNs        uint64
	PacketType       uint8

	// OK包字段
	AffectedRows     uint64
	LastInsertId     uint64
	StatusFlags      uint16
	Warnings         uint16
	Info             [256]byte

	// 错误包字段
	ErrorCode        uint16
	SqlState         [6]byte
	ErrorMessage     [512]byte
	ErrorMessageLen  uint16

	// 结果集信息
	FieldCount       uint32
	RowCount         uint32

	// 执行状态
	IsError          uint8
	IsOk             uint8
	IsEof            uint8
}

// MySQLTransaction MySQL事务
type MySQLTransaction struct {
	Handshake    MySQLHandshake
	Auth         MySQLAuth
	Command      MySQLCommand
	Response     MySQLResponse
	HasHandshake uint8
	HasAuth      uint8
	Complete     uint8
	Padding      uint8
}

// LoadMySQLFull 加载MySQL全字段解析eBPF程序
func LoadMySQLFull(opts *ebpf.CollectionOptions) (*MySQLFullObjects, error) {
	// 读取编译后的eBPF对象文件
	objData, err := mysqlFullBpfFS.ReadFile("mysql_full.bpf.o")
	if err != nil {
		return nil, fmt.Errorf("读取MySQL全字段解析eBPF对象失败: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(objData))
	if err != nil {
		return nil, fmt.Errorf("加载MySQL全字段解析eBPF规格失败: %w", err)
	}

	var objs MySQLFullObjects
	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("加载MySQL全字段解析eBPF对象失败: %w", err)
	}

	return &objs, nil
}

// Close 关闭所有eBPF对象
func (o *MySQLFullObjects) Close() error {
	var errs []error

	if o.TraceMySQLRecvmsg != nil {
		if err := o.TraceMySQLRecvmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceMySQLSendmsg != nil {
		if err := o.TraceMySQLSendmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceMySQLRecvmsgResponse != nil {
		if err := o.TraceMySQLRecvmsgResponse.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceMySQLTcpClose != nil {
		if err := o.TraceMySQLTcpClose.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if o.MySQLConnectionsMap != nil {
		if err := o.MySQLConnectionsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.MySQLEventsMap != nil {
		if err := o.MySQLEventsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.MySQLCmdStatsMap != nil {
		if err := o.MySQLCmdStatsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.MySQLErrorStatsMap != nil {
		if err := o.MySQLErrorStatsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭MySQL全字段解析eBPF对象时发生错误: %v", errs)
	}
	return nil
}

// AttachMySQLFullProbes 附加MySQL全字段解析kprobe探针
func AttachMySQLFullProbes(objs *MySQLFullObjects) ([]link.Link, error) {
	var links []link.Link

	// 附加tcp_recvmsg探针 (用于接收握手包)
	if objs.TraceMySQLRecvmsg != nil {
		l, err := link.Kprobe("tcp_recvmsg", objs.TraceMySQLRecvmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_recvmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_sendmsg探针 (用于发送命令)
	if objs.TraceMySQLSendmsg != nil {
		l, err := link.Kprobe("tcp_sendmsg", objs.TraceMySQLSendmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_sendmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_recvmsg探针 (用于接收响应) - 注意：这里可能需要使用不同的probe名称
	if objs.TraceMySQLRecvmsgResponse != nil {
		l, err := link.Kprobe("tcp_recvmsg", objs.TraceMySQLRecvmsgResponse, nil)
		if err != nil {
			// 如果失败，不视为致命错误，因为可能和上面的recvmsg冲突
			_ = err
		} else {
			links = append(links, l)
		}
	}

	// 附加tcp_close探针
	if objs.TraceMySQLTcpClose != nil {
		l, err := link.Kprobe("tcp_close", objs.TraceMySQLTcpClose, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_close kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	return links, nil
}

// GetMySQLTransaction 从事件队列中获取一个MySQL事务
func (o *MySQLFullObjects) GetMySQLTransaction() (*MySQLTransaction, error) {
	var txn MySQLTransaction
	err := o.MySQLEventsMap.LookupAndDelete(nil, &txn)
	if err != nil {
		return nil, fmt.Errorf("获取MySQL事务失败: %w", err)
	}
	return &txn, nil
}

// IterateMySQLConnections 遍历MySQL连接映射
func (o *MySQLFullObjects) IterateMySQLConnections() *ebpf.MapIterator {
	return o.MySQLConnectionsMap.Iterate()
}

// GetCommandStats 获取命令统计信息
func (o *MySQLFullObjects) GetCommandStats(cmdKey uint32) (uint64, error) {
	var count uint64
	err := o.MySQLCmdStatsMap.Lookup(&cmdKey, &count)
	if err != nil {
		return 0, fmt.Errorf("获取命令统计信息失败: %w", err)
	}
	return count, nil
}

// GetErrorStats 获取错误统计信息
func (o *MySQLFullObjects) GetErrorStats(errorCode uint16) (uint64, error) {
	var count uint64
	err := o.MySQLErrorStatsMap.Lookup(&errorCode, &count)
	if err != nil {
		return 0, fmt.Errorf("获取错误统计信息失败: %w", err)
	}
	return count, nil
}

// IterateErrorStats 遍历错误统计映射
func (o *MySQLFullObjects) IterateErrorStats() *ebpf.MapIterator {
	return o.MySQLErrorStatsMap.Iterate()
}

// GetCommandName 获取MySQL命令名称
func GetCommandName(cmd uint8) string {
	switch cmd {
	case MYSQL_COM_SLEEP:
		return "SLEEP"
	case MYSQL_COM_QUIT:
		return "QUIT"
	case MYSQL_COM_INIT_DB:
		return "INIT_DB"
	case MYSQL_COM_QUERY:
		return "QUERY"
	case MYSQL_COM_FIELD_LIST:
		return "FIELD_LIST"
	case MYSQL_COM_CREATE_DB:
		return "CREATE_DB"
	case MYSQL_COM_DROP_DB:
		return "DROP_DB"
	case MYSQL_COM_REFRESH:
		return "REFRESH"
	case MYSQL_COM_SHUTDOWN:
		return "SHUTDOWN"
	case MYSQL_COM_STATISTICS:
		return "STATISTICS"
	case MYSQL_COM_PROCESS_INFO:
		return "PROCESS_INFO"
	case MYSQL_COM_CONNECT:
		return "CONNECT"
	case MYSQL_COM_PROCESS_KILL:
		return "PROCESS_KILL"
	case MYSQL_COM_DEBUG:
		return "DEBUG"
	case MYSQL_COM_PING:
		return "PING"
	case MYSQL_COM_CHANGE_USER:
		return "CHANGE_USER"
	case MYSQL_COM_STMT_PREPARE:
		return "STMT_PREPARE"
	case MYSQL_COM_STMT_EXECUTE:
		return "STMT_EXECUTE"
	case MYSQL_COM_STMT_CLOSE:
		return "STMT_CLOSE"
	case MYSQL_COM_STMT_RESET:
		return "STMT_RESET"
	case MYSQL_COM_SET_OPTION:
		return "SET_OPTION"
	case MYSQL_COM_STMT_FETCH:
		return "STMT_FETCH"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", cmd)
	}
}

// GetPacketTypeName 获取MySQL包类型名称
func GetPacketTypeName(packetType uint8) string {
	switch packetType {
	case MYSQL_PACKET_OK:
		return "OK"
	case MYSQL_PACKET_ERR:
		return "ERR"
	case MYSQL_PACKET_EOF:
		return "EOF"
	case MYSQL_PACKET_AUTH:
		return "AUTH"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", packetType)
	}
}

// GetErrorName 获取MySQL错误名称
func GetErrorName(errorCode uint16) string {
	switch errorCode {
	case MYSQL_ERR_ACCESS_DENIED:
		return "ACCESS_DENIED"
	case MYSQL_ERR_BAD_DB:
		return "BAD_DB"
	case MYSQL_ERR_TABLE_EXISTS:
		return "TABLE_EXISTS"
	case MYSQL_ERR_BAD_TABLE:
		return "BAD_TABLE"
	case MYSQL_ERR_NO_SUCH_TABLE:
		return "NO_SUCH_TABLE"
	case MYSQL_ERR_PARSE_ERROR:
		return "PARSE_ERROR"
	case MYSQL_ERR_CONN_LOST:
		return "CONN_LOST"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", errorCode)
	}
}

// GetServerVersion 获取服务器版本字符串
func (h *MySQLHandshake) GetServerVersion() string {
	// 查找字符串结束位置
	for i := 0; i < len(h.ServerVersion); i++ {
		if h.ServerVersion[i] == 0 {
			return string(h.ServerVersion[:i])
		}
	}
	return string(h.ServerVersion[:])
}

// GetUsername 获取用户名
func (a *MySQLAuth) GetUsername() string {
	// 查找字符串结束位置
	for i := 0; i < len(a.Username); i++ {
		if a.Username[i] == 0 {
			return string(a.Username[:i])
		}
	}
	return string(a.Username[:])
}

// GetDatabase 获取数据库名
func (a *MySQLAuth) GetDatabase() string {
	// 查找字符串结束位置
	for i := 0; i < len(a.Database); i++ {
		if a.Database[i] == 0 {
			return string(a.Database[:i])
		}
	}
	return string(a.Database[:])
}

// GetSQL 获取SQL语句
func (c *MySQLCommand) GetSQL() string {
	if c.ArgumentLen == 0 || int(c.ArgumentLen) > len(c.Argument) {
		return ""
	}
	return string(c.Argument[:c.ArgumentLen])
}

// GetErrorMessage 获取错误消息
func (r *MySQLResponse) GetErrorMessage() string {
	if r.ErrorMessageLen == 0 || int(r.ErrorMessageLen) > len(r.ErrorMessage) {
		return ""
	}
	return string(r.ErrorMessage[:r.ErrorMessageLen])
}

// GetInfo 获取信息字符串
func (r *MySQLResponse) GetInfo() string {
	// 查找字符串结束位置
	for i := 0; i < len(r.Info); i++ {
		if r.Info[i] == 0 {
			return string(r.Info[:i])
		}
	}
	return string(r.Info[:])
}

// GetSQLState 获取SQL状态
func (r *MySQLResponse) GetSQLState() string {
	return string(r.SqlState[:])
}

// IsSuccess 检查MySQL命令是否成功执行
func (r *MySQLResponse) IsSuccess() bool {
	return r.IsOk == 1 || (r.IsError == 0 && r.PacketType == MYSQL_PACKET_OK)
}

// IsError 检查MySQL命令是否执行出错
func (r *MySQLResponse) IsErrorResponse() bool {
	return r.IsError == 1 || r.PacketType == MYSQL_PACKET_ERR
}

// GetTables 获取SQL中涉及的表名
func (c *MySQLCommand) GetTables() []string {
	var tables []string
	for i := 0; i < int(c.TableCount) && i < len(c.Tables); i++ {
		// 查找字符串结束位置
		tableName := ""
		for j := 0; j < len(c.Tables[i]); j++ {
			if c.Tables[i][j] == 0 {
				tableName = string(c.Tables[i][:j])
				break
			}
		}
		if tableName == "" {
			tableName = string(c.Tables[i][:])
		}
		tables = append(tables, tableName)
	}
	return tables
}

// GetSQLType 获取SQL语句类型描述
func (c *MySQLCommand) GetSQLType() string {
	if c.IsSelect == 1 {
		return "SELECT"
	}
	if c.IsInsert == 1 {
		return "INSERT"
	}
	if c.IsUpdate == 1 {
		return "UPDATE"
	}
	if c.IsDelete == 1 {
		return "DELETE"
	}
	if c.IsDDL == 1 {
		return "DDL"
	}
	if c.IsDCL == 1 {
		return "DCL"
	}
	if c.IsTransaction == 1 {
		return "TRANSACTION"
	}
	return "OTHER"
}
