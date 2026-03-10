<p align="center">
  <img src="https://img.shields.io/badge/Monorepo-CliRelay-0f172a?style=for-the-badge" alt="CliRelay Monorepo" />
  <img src="https://img.shields.io/badge/Backend-Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Frontend-React%20%2B%20TypeScript-2563eb?style=for-the-badge&logo=react&logoColor=white" alt="React TypeScript" />
  <img src="https://img.shields.io/badge/Runtime-Docker%20%2B%20Bun-16a34a?style=for-the-badge" alt="Docker and Bun" />
</p>

<h1 align="center">CliRelay Monorepo</h1>

<p align="center">
  将 <code>CliRelay</code> 后端与 <code>codeProxy</code> 前端管理台收口到同一仓库的二次开发版本。
</p>

<p align="center">
  <a href="#项目简介">项目简介</a> ·
  <a href="#来源与致谢">来源与致谢</a> ·
  <a href="#仓库结构">仓库结构</a> ·
  <a href="#与上游仓库的明确差异">功能差异</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#文档索引">文档索引</a>
</p>

## 项目简介

这个仓库用于统一维护一套完整的 AI 代理服务与管理后台：

- `CliRelay/`
  负责后端代理、认证、路由、日志、监控与配置管理。
- `codeProxy/`
  负责前端管理台、监控中心、供应商管理、API Key 管理、模型管理与认证文件管理。

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

这里的内容不是只看本地目录名整理出来的，而是基于当前仓库与上游两个仓库代码对比后的归纳。

### 后端新增或强化的内容

1. 管理端 usage / monitor 查询链路做了继续扩展
- 当前后端相比上游，`management` 相关处理器存在进一步差异
- 重点集中在 `dashboard_summary.go`、`usage.go`、`usage_logs_handler.go`、`server.go`
- 当前仓库额外包含 `monitor.go`、`usage_overview.go`、`usage_credential_health.go` 等管理侧处理逻辑

2. SQLite 统计查询能力继续调整
- 当前 [`usage_db.go`](./CliRelay/internal/usage/usage_db.go) 与上游存在差异
- 说明围绕监控页、日志页、仪表盘的数据查询与聚合逻辑做过后续调整
- 这部分也是当前监控中心、仪表盘和请求日志能联动的核心基础

3. 补了管理端 usage 日志相关测试与接口收口
- 当前仓库包含 [`usage_logs_handler_test.go`](./CliRelay/internal/api/handlers/management/usage_logs_handler_test.go)
- 表明请求日志接口这一层在当前版本里被单独强化过

4. 补充了 Codex 模型拉取能力
- 当前仓库额外包含 [`codex_models_fetch.go`](./CliRelay/internal/runtime/executor/codex_models_fetch.go)
- 同时包含对应测试 [`codex_models_fetch_test.go`](./CliRelay/internal/runtime/executor/codex_models_fetch_test.go)

### 前端新增或强化的内容

1. 监控中心相关页面与图表逻辑做过定向调整
- 当前仓库与上游相比，`components/monitor/*` 与 `modules/monitor/*` 下大量文件存在差异
- 包括 `ChannelStats`、`DailyTrendChart`、`FailureAnalysis`、`HourlyModelChart`、`KpiCards`、`RequestLogs`、`MonitorPage`
- 说明监控中心的数据结构、图表呈现和日志联动并不是简单沿用上游默认实现

2. 管理后台数据访问层做了适配
- 当前仓库的 `src/lib/http/apis/*` 多个文件和上游不同
- 包括 `api-call.ts`、`config.ts`、`logs.ts`、`models.ts`、`providers.ts`、`usage.ts`
- 同时当前仓库额外有 [`src/lib/http/transformers.ts`](./codeProxy/src/lib/http/transformers.ts)
- 这说明前端对后端接口返回结构做过进一步转换和收口

3. Auth Files / Models / API Keys / Providers 页面做过联动修改
- 当前仓库中这些页面均与上游存在差异：
  - [`AuthFilesPage.tsx`](./codeProxy/src/modules/auth-files/AuthFilesPage.tsx)
  - [`ModelsPage.tsx`](./codeProxy/src/modules/models/ModelsPage.tsx)
  - [`ApiKeysPage.tsx`](./codeProxy/src/modules/api-keys/ApiKeysPage.tsx)
  - [`ProvidersPage.tsx`](./codeProxy/src/modules/providers/ProvidersPage.tsx)
- 说明当前版本不是只改监控页，而是把多处管理页面与新的后端数据流一起调整过

4. usage / monitor 类型与索引逻辑做了额外收口
- 当前仓库额外包含 [`src/modules/monitor/types.ts`](./codeProxy/src/modules/monitor/types.ts)
- 以及 [`src/modules/usage/usageLogsIndex.ts`](./codeProxy/src/modules/usage/usageLogsIndex.ts)
- 说明前端在 usage 日志、监控查询和类型组织层做过专门整理

### 对整体工程形态的调整

1. 将原本两个独立仓库收口为单仓
- 当前仓库将后端 `CliRelay/` 与前端 `codeProxy/` 合并到同一个 Git 仓库
- 更适合统一联调、统一发版、统一文档维护

2. 前端接口层从旧 `services/api` 路径收口到当前 `lib/http/apis`
- 对比上游可见，上游仍保留较多 `src/services/api/*`
- 当前仓库实际以 `src/lib/http/apis/*` 为主要接口访问层
- 这属于一次比较明确的前端工程结构收口

3. 当前版本的重点不是“从零新增一套全新系统”，而是基于上游已存在的管理后台能力继续做联调适配、接口收口、数据查询修正与页面稳定性修复
- 这一点从大量“同名文件存在差异”而不是“整块目录完全新增”可以看出来
- 因此当前仓库更准确的定位是：在上游项目持续演进后的基础上，继续做面向实际使用场景的二次开发与整仓整合

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
