# gotunnel V2 设计文档

## V1 回顾

V1 实现了核心隧道功能：单连接多路复用、TCP 端口转发、心跳保活、断线重连、Cobra CLI。

## V2 目标

在 V1 基础上增加生产级特性，让 gotunnel 真正可用于日常开发。

## V2 功能规划

### Phase 1: 安全与可靠性（2 周）

#### 1.1 认证机制
- 客户端连接时发送 token 进行身份验证
- 服务端配置允许的 token 列表
- 未认证客户端无法注册隧道

**实现**:
- 新增 `AUTH` 消息类型 (0x07)
- 服务端启动时读取 `--token` 参数或环境变量
- 客户端通过 `--token` 参数传递

#### 1.2 TLS 加密
- 控制通道支持 TLS 加密
- 服务端配置证书文件
- 客户端可选择跳过证书验证（开发环境）

**实现**:
- 服务端: `--tls-cert` / `--tls-key` 参数
- 客户端: `--tls` / `--insecure` 参数

#### 1.3 流量限制
- 限制单客户端带宽（防止滥用）
- 限制最大并发连接数

### Phase 2: 功能增强（2 周）

#### 2.1 多隧道支持
- 一个客户端同时暴露多个本地服务
- 每个隧道独立配置

**CLI 示例**:
```bash
gotunnel client --server vps:7000 \
  --tunnel 3000:web \
  --tunnel 5432:db \
  --tunnel 6379:redis
```

#### 2.2 HTTP 模式
- 自动识别 HTTP 请求
- 注入 `X-Forwarded-For`、`X-Real-IP` 等 Header
- 支持 WebSocket 升级

#### 2.3 自定义子域名
- 为隧道分配固定子域名
- 需要配合 DNS 泛域名解析

**CLI 示例**:
```bash
gotunnel client --server vps:7000 --local 3000 --subdomain myapp
# 访问 myapp.gotunnel.dev → localhost:3000
```

### Phase 3: 可观测性（1 周）

#### 3.1 Web 控制台
- 简单的 Web UI 显示活跃隧道
- 实时连接数、流量统计
- 一键复制隧道地址

**实现**:
- 内嵌静态文件（Go embed）
- 端口: `--dashboard-port 8080`

#### 3.2 Prometheus 指标
- 暴露 `/metrics` 端点
- 指标: 连接数、流量、延迟、错误率

#### 3.3 结构化日志增强
- JSON 格式日志输出
- 请求级别的 trace ID

### Phase 4: 部署与运维（1 周）

#### 4.1 Docker 支持
- 提供 Dockerfile
- docker-compose.yml 示例

#### 4.2 配置文件
- YAML 配置文件支持
- 环境变量覆盖

**配置示例**:
```yaml
server:
  port: 7000
  port_range: 8000-9000
  token: "my-secret-token"
  tls:
    cert: /path/to/cert.pem
    key: /path/to/key.pem

client:
  server: vps:7000
  tunnels:
    - local: 3000
      remote: 0
      subdomain: myapp
```

#### 4.3 Systemd Service 文件
- 服务端/客户端的 systemd 单元文件
- 开机自启动

## 优先级排序

| 优先级 | 功能 | 理由 |
|--------|------|------|
| P0 | 认证机制 | 安全基础，无认证=裸奔 |
| P0 | TLS 加密 | 数据安全 |
| P1 | 多隧道支持 | 实用性大幅提升 |
| P1 | 配置文件 | 运维友好 |
| P2 | HTTP 模式 | Web 开发场景优化 |
| P2 | Web 控制台 | 可视化管理 |
| P3 | 自定义子域名 | 需要 DNS 配合，门槛高 |
| P3 | Prometheus 指标 | 监控体系集成 |

## 建议执行顺序

1. **Phase 1** (认证 + TLS + 配置文件) — 安全基础
2. **Phase 2** (多隧道 + HTTP 模式) — 核心功能增强
3. **Phase 4** (Docker + Systemd) — 部署便利性
4. **Phase 3** (Web 控制台 + Prometheus) — 可观测性

每个 Phase 可以独立发布版本：
- v2.0.0: 认证 + TLS
- v2.1.0: 多隧道 + HTTP 模式
- v2.2.0: Docker + 配置文件
- v2.3.0: Web 控制台 + 监控
