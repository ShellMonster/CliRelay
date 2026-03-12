# AI 部署指南

这份文档专门给 AI Agent 或“把仓库链接丢给 AI 让它自动部署”的场景使用。

在执行任何部署命令前，先看这份文档。

## 硬规则

1. 不要假设 `docker compose up -d` 就足够。
2. 不要在没有准备 `data/config.yaml` 的情况下启动容器。
3. 不要把 `http://localhost:8317/v1` 当成探活地址。
4. 不要把前端页面地址直接当成后端 API 地址，除非它们确实被反向代理到了同一个入口。
5. 如果用户要使用管理面板，必须设置 `remote-management.secret-key`。
6. 如果用户要从其他机器访问管理面板，还必须设置 `remote-management.allow-remote: true`。

## 正确的后端路径

- 服务根路径：`http://localhost:8317/`
- OpenAI 兼容 API Base：`http://localhost:8317/v1`
- 管理 API Base：`http://localhost:8317/v0/management`

重要说明：
- `GET /v1` 不是通用根接口，返回 404 可能是正常的。
- 判断服务是否活着，请访问 `/`。

## Docker 启动前必须准备的文件

Docker Compose 当前会用下面的命令启动后端：

```bash
./CLIProxyAPI -config /data/config.yaml
```

所以第一次启动前，必须先准备这个文件：

```bash
data/config.yaml
```

准备方式：

```bash
mkdir -p data
cp config.example.yaml data/config.yaml
```

## 管理面板的最小配置

如果用户要使用前端管理面板，至少要设置：

```yaml
remote-management:
  allow-remote: false
  secret-key: "your-management-key"
```

说明：
- 当 `secret-key` 为空时，所有 `/v0/management/*` 都会返回 `404`
- 当请求来自远程机器且 `allow-remote` 为 `false` 时，管理接口会返回 `403`

## 推荐的 Docker 命令

请显式使用本地源码构建：

```bash
docker compose up -d --build
```

如果目的是运行当前 clone 下来的源码，不要只依赖 `docker compose up -d`。

## 前端登录规则

如果使用 `codeProxy` 前端：

- 后端基础地址应填写：`http://localhost:8317`
- 不要填写：`http://localhost:8317/v1`
- 不要填写：`http://localhost:8317/v0/management`

前端会自动拼接管理 API 前缀。

## 反向代理要求

如果前后端通过反向代理统一入口部署，代理必须正确转发：

- `/v0/management/*` 到 CliRelay 后端
- `/v1/*` 到 CliRelay 后端

如果这两类路径没转发好，即使后端已经启动，前端也可能出现 404。

## 快速验证清单

部署后：

1. 先检查服务根路径：

```bash
curl http://localhost:8317/
```

2. 如果启用了管理接口，再带管理密钥检查：

```bash
curl -H "Authorization: Bearer your-management-key" \
  http://localhost:8317/v0/management/config
```

3. 如果使用前端，请这样登录：

- API Base：`http://localhost:8317`
- Management Key：`remote-management.secret-key` 中配置的值

## 常见失败原因

### 前端登录返回 404

通常是以下原因之一：
- 没有准备 `data/config.yaml`
- `remote-management.secret-key` 为空
- 前端填错了后端基础地址
- 反向代理没有转发 `/v0/management/*`

### 前端登录返回 403

通常是：
- `remote-management.allow-remote` 是 `false`
- 当前请求来自非 localhost 机器

### `http://localhost:8317/v1` 打不开

很多时候这是正常现象。

正确理解是：
- `http://localhost:8317/` 用于检查服务是否启动
- `http://localhost:8317/v1/...` 用于实际业务 API 调用

## 推荐的 AI 部署顺序

如果由 AI Agent 部署这个仓库，建议严格按下面顺序执行：

1. 从 `config.example.yaml` 复制生成 `data/config.yaml`
2. 如果需要管理面板，要求用户提供或设置 `remote-management.secret-key`
3. 补充可用的 API keys / auth 配置
4. 运行 `docker compose up -d --build`
5. 验证 `/`
6. 如果启用了管理接口，再验证 `/v0/management/config`
7. 前端登录时使用 `http://localhost:8317` 作为基础地址
