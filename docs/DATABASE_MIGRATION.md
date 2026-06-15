# CloudFlow 数据库国产化迁移指南

## 目录
1. [概述](#概述)
2. [支持的国产数据库清单](#支持的国产数据库清单)
3. [迁移前准备](#迁移前准备)
4. [5级双写模式迁移流程](#5级双写模式迁移流程)
5. [各数据库迁移详细步骤](#各数据库迁移详细步骤)
6. [数据校验方案](#数据校验方案)
7. [回滚方案](#回滚方案)
8. [常见问题排查](#常见问题排查)

---

## 概述

CloudFlow 采用**抽象层+双写兼容层**架构实现数据库国产化迁移，支持：
- ✅ 业务代码零修改
- ✅ 渐进式无中断迁移
- ✅ 随时可回滚
- ✅ SQL方言自动转换

---

## 支持的国产数据库清单

### 关系型数据库（RelationalStorage）
| 数据库 | 版本 | 兼容性 | 驱动状态 |
|--------|------|--------|----------|
| MySQL | 5.7+/8.0+ | 100% | ✅ 生产可用（回滚基准） |
| 达梦 DM8 | 8.1.2+ | 95% | ✅ 完整支持 |
| 人大金仓 KingBaseES | V8 R6+ | 98% | ✅ 完整支持 |
| 高斯 GaussDB | 3.0+ | 开发中 | 🔄 进行中 |
| OceanBase | 4.0+ | 开发中 | 🔄 进行中 |

### 时序数据库（TimeSeriesStorage）
| 数据库 | 版本 | 兼容性 | 驱动状态 |
|--------|------|--------|----------|
| ClickHouse | 22.8+ | 100% | ✅ 生产可用 |
| 达梦 DM8 时序版 | 8.1.2+ | 90% | ✅ 完整支持 |

### KV存储（KVStorage）
| 数据库 | 版本 | 兼容性 | 驱动状态 |
|--------|------|--------|----------|
| Redis | 6.0+/7.0+ | 100% | ✅ 生产可用 |
| GaussDB(for Redis) | 3.0+ | 100% | ✅ 完整支持 |

---

## 迁移前准备

### 1. 环境检查
```bash
# 检查数据库连通性
# MySQL
mysql -h host -P port -u user -p

# 达梦DM8
disql user/password@host:port

# 人大金仓
ksql -h host -p port -U user -d database
```

### 2. 数据库初始化
```sql
-- 达梦DM8 创建表空间和用户
CREATE TABLESPACE CLOUDFLOW DATAFILE 'CLOUDFLOW.DBF' SIZE 10240;
CREATE USER CLOUDFLOW IDENTIFIED BY "YourPassword" DEFAULT TABLESPACE CLOUDFLOW;
GRANT DBA TO CLOUDFLOW;

-- 人大金仓 创建用户
CREATE USER cloudflow WITH PASSWORD 'YourPassword';
CREATE DATABASE cloudflow OWNER cloudflow;
GRANT ALL PRIVILEGES ON DATABASE cloudflow TO cloudflow;
```

### 3. 配置文件准备
复制 `config.yaml` 到部署目录，根据目标数据库修改配置。

---

## 5级双写模式迁移流程

### 迁移阶段总览
```
Phase 1: 准备 → Phase 2: 预热 → Phase 3: 同步 → Phase 4: 验证 → Phase 5: 切流
   Mode 0       Mode 1       Mode 2       Mode 3       Mode 4
```

### Level 0: ModeOldOnly - 仅写旧库
**状态**: 迁移准备阶段
```yaml
relational_db:
  enable_dual_write: true
  dual_write_mode: 0    # 仅写旧库
  secondary_type: dameng
  secondary_host: 127.0.0.1
  secondary_port: 5236
```

**操作**:
1. 部署新库环境
2. 创建表结构（DDL自动转换）
3. 验证新库连通性

**验收标准**: 新库可正常连接，表结构创建成功

---

### Level 1: ModeAsyncWrite - 异步双写
**状态**: 数据预热阶段
```yaml
dual_write_mode: 1    # 异步双写
```

**操作**:
1. 开启异步双写
2. 运行7天进行数据预热
3. 监控从库写入延迟

**监控指标**:
- 主库写入性能下降 < 5%
- 从库延迟 < 1分钟
- 双写错误率 = 0

**验收标准**: 预热7天后，新旧库数据差异 < 0.1%

---

### Level 2: ModeSyncWrite - 同步双写
**状态**: 数据一致性保障阶段
```yaml
dual_write_mode: 2    # 同步双写
```

**操作**:
1. 切换到同步双写模式
2. 全量数据校验
3. 修复不一致数据

**监控指标**:
- 主库写入性能下降 < 15%
- 双写错误率 = 0
- 数据一致性 = 100%

**验收标准**: 连续运行3天，数据100%一致

---

### Level 3: ModeReadSplit - 读流量切分
**状态**: 新库验证阶段
```yaml
dual_write_mode: 3    # 读流量切分
```

**操作（灰度发布）**:
1. **Day 1**: 1% 读流量切到新库
2. **Day 2**: 10% 读流量切到新库
3. **Day 3**: 50% 读流量切到新库
4. **Day 4-7**: 100% 读流量切到新库

**验证内容**:
- 查询正确性（对比新旧库结果）
- 查询性能（P99延迟）
- 错误率监控

**验收标准**: 100%读流量运行7天，无异常

---

### Level 4: ModeNewOnly - 仅写新库
**状态**: 迁移完成阶段
```yaml
dual_write_mode: 4    # 仅写新库
```

**操作**:
1. 完全切到新库
2. 旧库保留只读用于回滚
3. 观察期7-14天

**验收标准**: 新库独立运行14天，性能稳定

---

## 各数据库迁移详细步骤

### MySQL → 达梦DM8 迁移

#### 1. SQL兼容说明
| MySQL | 达梦 | 自动转换 |
|-------|------|----------|
| `` `column` `` | `"column"` | ✅ |
| `IFNULL(a, b)` | `NVL(a, b)` | ✅ |
| `NOW()` | `SYSDATE` | ✅ |
| `AUTO_INCREMENT` | `IDENTITY(1,1)` | ✅ |
| `UNSIGNED` | 移除 | ✅ |
| `ENGINE=InnoDB` | 移除 | ✅ |
| `GROUP_CONCAT` | `LISTAGG` | ✅ |
| `LIMIT n OFFSET m` | 原生支持 | ✅ |

#### 2. 特殊语法处理
- `ON DUPLICATE KEY UPDATE` → 使用 `MERGE INTO`
- 存储过程需要手动转换

---

### MySQL → 人大金仓 KingBaseES 迁移

#### 1. SQL兼容说明
KingBaseES 98% 兼容 MySQL 语法，仅需少量转换：
- `IFNULL` → `NVL`（可选）
- 其他语法原生支持

---

## 数据校验方案

### 1. 行数校验
```sql
-- 旧库（MySQL）
SELECT table_name, table_rows 
FROM information_schema.tables 
WHERE table_schema = 'cloudflow';

-- 新库（达梦）
SELECT table_name, count(*) 
FROM user_tables ut, all_tab_statistics ats
WHERE ut.table_name = ats.table_name;
```

### 2. 关键表抽样校验
```sql
-- 对比最近1小时数据
SELECT COUNT(*), SUM(bytes), MAX(timestamp)
FROM flows
WHERE timestamp > NOW() - INTERVAL 1 HOUR;
```

### 3. 自动化校验脚本
```bash
# 运行数据校验工具
./scripts/validate_db.sh --source mysql --target dameng
```

---

## 回滚方案

### 紧急回滚（5分钟内完成）
```yaml
# 立即切回旧库
relational_db:
  enable_dual_write: false
  dual_write_mode: 0
```

### 回滚验证
1. 验证应用连接正常
2. 验证读写功能正常
3. 验证数据完整性

### 回滚触发条件
- 新库错误率 > 0.1% 持续5分钟
- 新库P99延迟 > 旧库2倍
- 数据一致性 < 99.9%

---

## 常见问题排查

### Q1: 达梦连接失败
**症状**: `invalid username/password`
**解决**:
1. 达梦用户名密码大小写敏感
2. 确认端口5236是否开放
3. 检查防火墙配置

### Q2: 双写性能下降
**症状**: 写入延迟增加
**解决**:
1. 检查网络延迟（建议同机房部署）
2. 调整连接池参数
3. Mode 1异步模式性能影响最小

### Q3: SQL执行报错
**症状**: `语法错误`
**解决**:
1. 查看日志中的原始SQL
2. 确认方言转换是否覆盖
3. 特殊SQL手动适配

### Q4: 数据不一致
**症状**: 新旧库数据有差异
**解决**:
1. 检查双写错误日志
2. 运行修复脚本补全数据
3. 确认没有批量导入绕过双写层

---

## 联系支持

迁移过程中遇到问题，请：
1. 查看应用日志中的数据库错误
2. 检查各数据库的慢查询日志
3. 提交 Issue 并附上相关日志
