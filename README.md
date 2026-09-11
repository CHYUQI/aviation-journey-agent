# 基于Agent的民航旅客智能出行系统

面向民航旅客起飞前行程规划与延误风险预警的课程设计项目。

## 架构概要

- 单个 Go 二进制承载 Gin API、Journey Runtime、DataAgent、AdviceAgent 和 SSE。
- Vue 3 + TypeScript 单页仪表盘。
- DataAgent 通过 Skill Registry 查询、标准化和融合真实数据。
- DataAgent 与 AdviceAgent 均通过调用大模型完成状态理解与行动决策，并由提示词、数据技能与工具约束其输出。
- 不接入消息队列、独立模型服务、WebSocket 或前端地图 SDK。
- 城市导航使用高德 URI；机场内部使用时间轴和文字指引。

## 开发运行

### 后端

```powershell
cd backend
go mod tidy
go run ./cmd/server
```

默认监听 `http://localhost:8080`。

### 前端

```powershell
cd frontend
npm install
npm run dev
```

默认监听 `http://localhost:5173`，开发服务器将 `/api` 代理到后端。

## 接口契约

前后端并行开发以契约为准，不要互相读代码：

- `docs/api/openapi.yaml` — 机器可读的唯一真源
- `docs/api/contract.md` — 人工阅读版（枚举、空值语义、SSE 约定、变更流程）

契约变更后重新生成前端类型：

```powershell
cd frontend
npm run gen:api
```

前端不依赖后端时，用 mock 服务开发：

```powershell
cd backend
go run ./cmd/mockserver      # :8081
```

```powershell
cd frontend
$env:MOCK_API=1; npm run dev
```

详细实施顺序见 [projectplan.md](./projectplan.md)。



