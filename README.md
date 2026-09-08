# 航旅智行（Aviation Journey Agent）

面向民航旅客起飞前行程规划与延误风险预警的课程设计项目。

## 架构概要

- 单个 Go 二进制承载 Gin API、Journey Runtime、DataAgent、AdviceAgent 和 SSE。
- Vue 3 + TypeScript 单页仪表盘。
- DataAgent 通过 Skill Registry 查询、标准化和融合真实数据。
- AdviceAgent 只消费 `WorldState`，使用可解释的硬约束与规则生成行动建议。
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

详细实施顺序和接口契约见 [projectplan.md](./projectplan.md)。
