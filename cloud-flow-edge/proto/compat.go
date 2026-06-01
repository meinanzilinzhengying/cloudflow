// Package edge — proto v1 兼容层
// 让手写结构体兼容 gRPC 的 protobuf 编解码
package edge

import (
	"fmt"

	legacyproto "github.com/golang/protobuf/proto"
	"google.golang.org/grpc/encoding"
)

// init 注册 legacy codec。
// 此 codec 用于兼容基于 github.com/golang/protobuf 的旧版序列化路径。
// 当项目中存在手写 proto 结构体（未使用 protoc 生成）且需要通过 gRPC 传输时，
// 需要此 codec 来绕过 protobuf v2 对 ProtoReflect() 的要求。
// 如果确认所有 proto 结构体已迁移到 google.golang.org/protobuf 且不再需要
// github.com/golang/protobuf 兼容，可以注释掉此 init() 函数。
// TODO(AE-L04): 评估迁移到 google.golang.org/protobuf 的可行性，移除对 github.com/golang/protobuf 的依赖。
func init() {
	encoding.RegisterCodec(legacyCodec{})
}

// legacyCodec 使用 github.com/golang/protobuf 做序列化
// 绕过 protobuf v2 对 ProtoReflect() 的要求
type legacyCodec struct{}

func (legacyCodec) Marshal(v interface{}) ([]byte, error) {
	msg, ok := v.(legacyproto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
	}
	return legacyproto.Marshal(msg)
}

func (legacyCodec) Unmarshal(data []byte, v interface{}) error {
	msg, ok := v.(legacyproto.Message)
	if !ok {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	return legacyproto.Unmarshal(data, msg)
}

// Name 返回 codec 名称
// 使用明确的自定义名称 "cloudflow-legacy-proto"，避免与标准 proto codec 冲突
// 仅在需要兼容 github.com/golang/protobuf 的旧版序列化时使用此 codec
func (legacyCodec) Name() string {
	return "cloudflow-legacy-proto"
}
