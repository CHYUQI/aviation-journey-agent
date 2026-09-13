# 基于Agent的民航旅客智能出行系统

面向民航旅客起飞前行程规划与延误风险预警的课程设计项目。

## 架构概要

- 单个 Go 二进制承载 Gin API、Journey Runtime、DataAgent、AdviceAgent 和 SSE。
- Vue 3 + TypeScript 单页仪表盘。
- DataAgent 通过 Skill Registry 获取数据，标准化成 `WorldState`；
  AdviceAgent 只消费状态，调用大模型生成阶段判断与行动建议。
- 数据来源是**航司官网**：用本机 Edge 无头模式渲染 JS 页面并查询，
  把结果页文本交给模型抽取字段，再回原文校验。不接航班 API、不接地图 API。
- 不引入消息队列、独立模型服务、WebSocket、数据库或前端地图 SDK。
- 城市导航只拼一个高德跳转链接；机场内部用时间轴和文字指引。

## 快速开始

### 1. 配置

```powershell
cd backend
copy .env.example .env      # 填入 MODEL_API_KEY
go run ./cmd/modelcheck "你好"    # 验证模型能连通
```

`.env` 已被 gitignore，不会进版本库。

### 2. 运行

```powershell
# 终端 1：后端
cd backend
go run ./cmd/server          # :8080

# 终端 2：前端
cd frontend
npm run dev                  # :5173，/api 代理到 :8080
```

没有 API key 也能启动：Agent 会走降级路径，返回 `risk = unknown` 并在依据里说明原因。

### 3. 前端不依赖后端时

```powershell
cd backend
go run ./cmd/mockserver      # :8081
```

```powershell
cd frontend
$env:MOCK_API=1; npm run dev
```

## 接口契约

前后端并行开发以契约为准，不要互相读代码：

- `docs/api/openapi.yaml` — 机器可读的唯一真源
- `docs/api/contract.md` — 人工阅读版（枚举、空值语义、SSE 约定、变更流程）

契约变更后重新生成前端类型：

```powershell
cd frontend
npm run gen:api
```

## 调试工具

| 命令 | 用途 |
|---|---|
| `go run ./cmd/modelcheck "你好"` | 验证模型端点、key、模型名是否可用 |
| `go run ./cmd/modelcheck -json "输出 JSON"` | 验证 JSON 模式 |
| `go run ./cmd/browsercheck -url <网址>` | 打开网页，列出输入框和按钮（调新航司配方用） |
| `go run ./cmd/browsercheck CZ3101` | 按配方查一次航班，打印结果页文本 |
| `go run ./cmd/skillcheck CZ3101 2026-09-13` | 跑完整航班技能，打印原始观测值与问题 |
| `go run ./cmd/mockserver` | 前端联调用的假后端 |

## 已知限制

- **航班动态只对已实测通过的航司可用**（见 `internal/skill/airline.go` 的配方表）。
  没在表里的航司不是"不支持"，而是"配方还没验证过"。
- **两家航司的实测情况**：南航可用；东航页面可操作但查询结果待定性；
  国航官网有滑块验证码，自动化会被拦，需要走手动兜底。
- 一次航班查询约 15 秒（浏览器渲染 + 模型抽取），不追求低延迟。
- 路程时间（`etaMin`）和机场内耗时目前没有数据源，相应字段留空。

详细实施顺序见 [projectplan.md](./projectplan.md)。
