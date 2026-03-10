# CliRelay 单仓版说明

这个仓库将原本分开的后端 `CliRelay/` 与前端 `codeProxy/` 合并到了同一个仓库中，便于统一维护、联调、部署与版本管理。

## 项目组成

- `CliRelay/`
  后端服务，负责 AI 请求代理、认证、路由、配置管理、监控与日志采集。
- `codeProxy/`
  前端管理台，负责监控中心、供应商管理、API Key 管理、模型管理、认证文件管理等页面。

## 来源与致谢

### 后端来源

后端目录 `CliRelay/` 来源于：

- <https://github.com/kittors/CliRelay>

当前仓库中的后端是在原项目基础上的二次开发版本。  
感谢原作者与贡献者提供的基础能力、代理架构与开源实现。

### 前端来源

前端目录 `codeProxy/` 来源于：

- <https://github.com/kittors/codeProxy>

当前仓库中的前端同样是在原项目基础上的二次开发版本，并与本仓库中的后端做了联动适配。

## 当前仓库相对原仓库的明确功能差异

下面这部分按“后端新增能力 / 前端新增能力 / 对原能力的重构调整”来整理，便于快速看清当前仓库与原始两个上游仓库相比，已经额外做了哪些事情。

### 后端新增能力

1. 新增 SQLite 持久化请求日志与统计聚合

- 当前后端包含独立的 `internal/usage` 模块，并使用 `modernc.org/sqlite`
- 请求日志、Token 统计、模型统计、仪表盘摘要不再只是运行期内存数据，而是可以落到 SQLite
- 已支持按时间范围、按 API Key、按模型查询，并生成仪表盘与监控中心需要的聚合结果

2. 新增面向管理后台的统计接口体系

- 后端已不仅提供基础代理能力，还补充了完整的管理端统计数据支撑
- 当前前端实际依赖的能力包括：仪表盘摘要、模型分布、按天趋势、按小时模型统计、请求日志、日志内容查看等
- 这意味着后端已经扩展出一套专门服务管理面板的数据接口，而不是只暴露原始 usage 汇总

3. 新增监控与运维友好的数据组织方式

- 监控中心查询的数据已经可以直接从 SQLite 明细或聚合结果中拿到
- 这使得重启后历史数据可恢复，且可以按近 7 天、近 30 天这类时间窗稳定查询

4. 新增配置变更 diff / watcher 机制

- 当前后端包含 `internal/watcher` 与 `internal/watcher/diff`
- 已覆盖模型列表、排除模型、OAuth 排除模型、OpenAI 兼容模型等配置差异检测
- 这类能力属于管理后台和配置审计场景的增强，不是基础代理功能自带的部分

5. 新增管理面板资源承载相关模块

- 当前代码中有 `internal/managementasset`
- 说明后端已经为管理端静态资源或面板集成做过专门支持

6. 新增 Amp 模块能力

- 当前后端有独立的 `internal/api/modules/amp`
- 包括路由、代理、fallback、模型映射、响应改写等模块
- 这是原始基础代理之外进一步扩展出的兼容能力

### 前端新增能力

1. 新增独立的监控中心与请求日志体系

- 当前前端有独立的 `modules/monitor`
- 已拆分为监控首页、请求日志页、错误详情弹窗、日志内容弹窗、图表配置与格式化工具
- 相比基础管理面板，当前监控中心已经是完整的分析型页面

2. 新增 API Key 管理页面

- 当前存在独立的 `modules/api-keys/ApiKeysPage.tsx`
- 支持把 API Key 管理从配置编辑中拆出来做单独操作界面

3. 新增模型管理页面

- 当前存在独立的 `modules/models/ModelsPage.tsx`
- 模型查看、路由或映射管理不再完全依赖手工 YAML 编辑

4. 新增认证文件管理页面

- 当前存在独立的 `modules/auth-files/AuthFilesPage.tsx`
- 并带有 OAuth 排除项、模型别名等相关管理能力

5. 新增配额管理页面

- 当前存在独立的 `modules/quota/QuotaPage.tsx`
- 说明前端已经扩展出与额度、配额或限制相关的可视化管理能力

6. 新增公开 API Key 查询页面

- 当前存在 `modules/apikey-lookup/ApiKeyLookupPage.tsx`
- 终端用户可以走公开页面查看自身的使用情况和日志，不必进入完整管理后台

7. 新增更完整的后台 UI 基础设施

- 当前前端包含 `VirtualTable`、`SearchableSelect`、`MultiSelect`、`ToastProvider`、`ThemeProvider`、`ConfirmModal` 等可复用基础组件
- 这些能力支撑了大数据量日志、可搜索下拉、多选、全局提示、主题切换等后台交互

### 对原能力的重构调整

1. 将前后端从两个独立仓库收口为单仓结构

- 当前仓库把后端 `CliRelay/` 与前端 `codeProxy/` 放到了同一个仓库中统一维护
- 更适合联调、统一发版、统一文档和统一忽略规则管理

2. 将前端页面结构重构为模块化组织

- 当前前端同时使用 `src/app`、`src/modules`、`src/router`、`src/modules/ui`
- 页面、路由、认证、HTTP 层、通用 UI 已经明显解耦
- 这比原始页面式堆叠更适合长期维护和持续扩展

3. 将监控页从旧 usage 兼容思路收口到新接口模型

- 当前代码已经围绕新的管理端监控接口组织页面与数据结构
- 监控中心、请求日志、仪表盘等能力不再依赖单一旧 usage 结构硬凑

4. 将部分配置能力从“直接改 YAML”调整为“页面化管理 + 配置编辑并存”

- 当前前端同时保留 `config` 页面与多个独立管理页面
- 也就是说，复杂配置仍可通过 YAML 方式处理，但日常运维项已经逐步拆成独立页面

5. 将部分路由与旧入口做了兼容整理

- 当前前端路由中既有新的页面入口，也保留了若干旧路径跳转
- 这类处理说明项目已经在做从旧页面结构到新页面结构的迁移收口

## 当前目录结构

```text
CliRelay/
├── README.md
├── .gitignore
├── CliRelay/        # 后端（Go）
└── codeProxy/       # 前端管理台（React + TypeScript）
```

## 开发建议

### 后端

```bash
cd CliRelay
go run ./cmd/cli-proxy-api
```

或按项目内 `docker-compose.yml` / Docker 方式启动。

### 前端

```bash
cd codeProxy
bun install
bun run dev
```

默认开发访问地址通常为：

- 前端管理台：`http://localhost:5273`
- 后端管理接口：`http://localhost:8317/v0/management`

## 说明

这个 README 主要说明当前“总仓库”的结构、来源与二次开发范围。  
各子目录中更细的功能介绍、接口说明与部署细节，仍以各自目录下文档为准：

- [后端中文说明](./CliRelay/README_CN.md)
- [后端英文说明](./CliRelay/README.md)
- [前端说明](./codeProxy/README.md)
