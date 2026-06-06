# SSH 远程安装探针 API 规范

## 概述
前端通过调用后端 API，后端执行 SSH 连接到远程服务器并自动安装探针。

## API 接口

### 1. SSH 安装探针

**Endpoint**: `POST /api/v1/probe/ssh-install`

**Request Body**:
```json
{
  "host": "192.168.1.100",
  "port": 22,
  "username": "root",
  "auth_type": "password",  // 或 "key"
  "password": "your-password",  // 密码认证时
  "private_key": "-----BEGIN RSA PRIVATE KEY-----...",  // 密钥认证时
  "probe_name": "agent-prod-01",
  "probe_type": "agent",  // agent, edge
  "group": "华北",
  "edge_addr": "edge:50051"
}
```

**Response (成功)**:
```json
{
  "code": 0,
  "message": "安装成功",
  "data": {
    "probe_id": "probe-xxx-xxx",
    "probe_name": "agent-prod-01",
    "ip": "192.168.1.100",
    "status": "online"
  }
}
```

**Response (失败)**:
```json
{
  "code": 1001,
  "message": "SSH 连接失败: Connection refused",
  "data": null
}
```

**错误码**:
- `1001`: SSH 连接失败
- `1002`: 认证失败
- `1003`: 安装脚本执行失败
- `1004`: 参数验证失败

## 后端实现建议

### 使用 Go SSH 库
推荐使用 `github.com/pkg/sftp` + `golang.org/x/crypto/ssh`

### 示例代码结构
```go
// cmd/probe/ssh_install.go
package main

import (
    "fmt"
    "net"
    "time"
    
    "github.com/pkg/sftp"
    "golang.org/x/crypto/ssh"
)

type SSHInstallRequest struct {
    Host       string `json:"host"`
    Port       int    `json:"port"`
    Username   string `json:"username"`
    AuthType   string `json:"auth_type"` // password or key
    Password   string `json:"password"`
    PrivateKey string `json:"private_key"`
    ProbeName  string `json:"probe_name"`
    ProbeType  string `json:"probe_type"`
    Group      string `json:"group"`
    EdgeAddr   string `json:"edge_addr"`
}

func SSHInstall(req SSHInstallRequest) error {
    // 1. 建立 SSH 连接
    config := &ssh.ClientConfig{
        User: req.Username,
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout: 30 * time.Second,
    }
    
    if req.AuthType == "password" {
        config.Auth = []ssh.AuthMethod{
            ssh.Password(req.Password),
        }
    } else {
        signer, _ := ssh.ParsePrivateKey([]byte(req.PrivateKey))
        config.Auth = []ssh.AuthMethod{
            ssh.PublicKeys(signer),
        }
    }
    
    addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
    client, err := ssh.Dial("tcp", addr, config)
    if err != nil {
        return fmt.Errorf("SSH连接失败: %v", err)
    }
    defer client.Close()
    
    // 2. 创建 SFTP 客户端
    sftpClient, err := sftp.NewClient(client)
    if err != nil {
        return fmt.Errorf("SFTP客户端创建失败: %v", err)
    }
    defer sftpClient.Close()
    
    // 3. 下载并执行安装脚本
    installScript := `#!/bin/bash
curl -sSL https://install.cloudflow.io/probe.sh | sh -s -- \
  --name=%s \
  --type=%s \
  --edge-addr=%s
`
    
    // ... 执行远程命令
    
    return nil
}
```

## 安全考虑

1. **密码传输**: 建议使用 HTTPS 传输密码
2. **密钥管理**: 私钥应加密存储
3. **权限控制**: 仅管理员可调用此 API
4. **审计日志**: 记录所有 SSH 安装操作
5. **连接超时**: 设置合理的超时时间
6. **并发限制**: 限制同时进行的 SSH 安装数量
