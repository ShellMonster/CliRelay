# CliRelay Monorepo

> 将 `CliRelay` 后端与 `codeProxy` 前端管理台收口到同一仓库的二次开发版本。

## 项目简介

这个仓库用于统一维护一套完整的 AI 代理服务与管理后台：

- `CliRelay/` 负责后端代理、认证、路由、日志、监控与配置管理
- `codeProxy/` 负责前端管理台、监控中心、供应商管理、API Key 管理、模型管理与认证文件管理

相较于“前后端分别一个仓库”的形态，当前单仓结构更适合：

- 前后端联调
- 统一发版与部署
- 统一文档与 Git 管理
- 围绕管理后台的数据接口持续演进

## 来源与致谢

### 后端

后端目录 [`CliRelay/`](./CliRelay) 基于 [kittors/CliRelay](https://github.com/kittors/CliRelay) 二次开发。  
感谢原作者与贡献者提供的开源基础能力、代理架构与认证实现。

### 前端

前端目录 [`codeProxy/`](./codeProxy) 基于 [kittors/codeProxy](https://github.com/kittors/codeProxy) 二次开发。  
当前仓库中的前端已和本仓库内后端做联动适配与结构调整。

## 仓库结构

```text
CliRelay/
├── README.md
├── .gitignore
├── CliRelay/      # 后端（Go）
└── codeProxy/     # 前端管理台（React + TypeScript）
```

## 与上游仓库的明确差异

下面不是泛泛而谈的“优化方向”，而是基于当前代码结构整理出来的明确功能差异。

### 后端新增能力

1. SQLite 持久化请求日志与聚合统计

- 当前后端包含独立的 `internal/usage` 模块，并使用 `modernc.org/sqlite`
- 请求日志、Token 统计、模型统计、仪表盘摘要不再只是内存态数据
- 已支撑按时间范围、按 API Key、按模型的监控查询与统计聚合

2. 面向管理后台的统计接口体系

- 当前后端除了基础代理接口，还提供了专门服务管理台的数据接口
- 已覆盖仪表盘摘要、模型分布、按天趋势、按小时模型统计、请求日志、日志内容查看等场景

3. 更稳定的历史数据查询能力

- 监控与仪表盘数据可直接从 SQLite 明细与聚合中获取
- 支持近 7 天、近 30 天等时间窗的稳定查询
- 避免了仅依赖内存态统计带来的重启丢失问题

4. 配置 diff / watcher 机制

- 当前后端包含 `internal/watcher` 与 `internal/watcher/diff`
- 已覆盖模型列表、排除模型、OAuth 排除模型、OpenAI 兼容模型等配置项差异检测

5. 管理面板资源承载能力

- 当前代码中存在 `internal/managementasset`
- 表明后端已经为管理端资源承载或面板集成做过专门处理

6. Amp 模块扩展

- 当前后端包含独立的 `internal/api/modules/amp`
- 已扩展路由、代理、fallback、模型映射、响应改写等能力

### 前端新增能力

1. 独立的监控中心与请求日志体系

- 当前前端包含独立的 `modules/monitor`
- 已拆分为监控页、请求日志页、错误详情弹窗、日志内容弹窗、图表配置与格式化工具

2. 独立的 API Key 管理页面

- 当前存在 [`ApiKeysPage.tsx`](./codeProxy/src/modules/api-keys/ApiKeysPage.tsx)
- API Key 管理不再只是附着在配置编辑中

3. 独立的模型管理页面

- 当前存在 [`ModelsPage.tsx`](./codeProxy/src/modules/models/ModelsPage.tsx)
- 模型管理不再完全依赖手工 YAML 编辑

4. 独立的认证文件管理页面

- 当前存在 [`AuthFilesPage.tsx`](./codeProxy/src/modules/auth-files/AuthFilesPage.tsx)
- 并带有 OAuth 排除项、模型别名等相关管理能力

5. 配额管理页面

- 当前存在 [`QuotaPage.tsx`](./codeProxy/src/modules/quota/QuotaPage.tsx)
- 已扩展出可视化的额度/限制管理能力

6. 公开 API Key 查询页面

- 当前存在 [`ApiKeyLookupPage.tsx`](./codeProxy/src/modules/apikey-lookup/ApiKeyLookupPage.tsx)
- 终端用户可在公开页面查询自身使用情况与请求日志

7. 更完整的后台 UI 基础设施

- 当前前端包含 `VirtualTable`、`SearchableSelect`、`MultiSelect`、`ToastProvider`、`ThemeProvider`、`ConfirmModal` 等通用模块
- 已明显从“页面拼装型前端”演进到“可复用后台组件体系”

### 对原能力的重构调整

1. 双仓收口为单仓

- 当前仓库将后端 `CliRelay/` 与前端 `codeProxy/` 合并到同一个 Git 仓库
- 更利于统一联调、统一发版与统一文档维护

2. 前端模块化重构

- 当前前端采用 `src/app`、`src/modules`、`src/router`、`src/modules/ui` 的组织方式
- 页面、路由、认证、HTTP 层与通用 UI 已经明显解耦

3. 监控页从旧 usage 兼容逻辑收口到新接口模型

- 当前代码已经围绕新的管理端监控接口组织页面与数据结构
- 监控中心、请求日志、仪表盘不再依赖单一旧 usage 结构硬凑

4. 从“直接改 YAML”演进到“页面化管理 + 配置编辑并存”

- 当前前端同时保留 `config` 页面与多个独立管理页面
- 复杂配置仍可通过 YAML 处理，日常运维项则逐步页面化

5. 新旧路由兼容整理

- 当前前端既有新的页面入口，也保留了部分旧路径跳转
- 已处于从旧页面结构向新页面结构迁移收口的状态

## 快速开始

### 后端：Docker 启动

```bash
cd CliRelay
cp config.example.yaml config.yaml
docker compose up -d
```

如在国内网络环境下需要镜像加速：

```bash
cd CliRelay
cp .env.china-mirror .env
cp config.example.yaml config.yaml
docker compose up -d
```

默认管理接口：

- `http://localhost:8317/v0/management`

### 前端：Bun 本地开发

```bash
cd codeProxy
bun install
bun run dev
```

默认访问地址：

- `http://localhost:5273`

## 文档索引

- [后端中文文档](./CliRelay/README_CN.md)
- [后端英文文档](./CliRelay/README.md)
- [前端文档](./codeProxy/README.md)

## 说明

这个 README 主要承担总仓库入口说明的职责：

- 说明当前单仓结构
- 标注上游来源与致谢
- 概括当前版本与原仓库的差异
- 给出统一的启动入口

更细的接口、部署和功能说明，仍以子目录文档为准。
