# 基于Agent的民航旅客智能出行系统

面向民航旅客起飞前行程规划与延误风险预警的课程设计项目。

## 架构概要

- 单个 Go 二进制承载 Gin API、Journey Runtime、DataAgent、AdviceAgent 和 SSE。
- Vue 3 + TypeScript 单页仪表盘。
- DataAgent 通过 Skill Registry 获取数据，标准化成 `WorldState`；
  AdviceAgent 只消费状态，调用大模型生成阶段判断与行动建议。
- 数据来源是 **EOOB**（聚合站）：航班动态走它的状态 JSON 接口，机场信息走机场页，
  全部是普通 HTTP 请求 + 客户端提示头。**不使用无头浏览器、不抓航司官网**：
  实测 EOOB 的 Cloudflare 对 headless 一律拦截，航司官网配方也同样被挡。
- 不引入消息队列、独立模型服务、WebSocket、数据库或前端地图 SDK。
- 城市导航只拼一个高德跳转链接；机场信息用时间轴与数据源原文文本行展示。

## 快速开始

### 1. 配置

```powershell
cd backend
copy .env.example .env      # 填入 MODEL_API_KEY
go run ./cmd/modelcheck "你好"    # 验证模型能连通
```

`.env` 已被 gitignore，不会进版本库。

`.env` 的读取不依赖启动目录：服务会从当前目录向上找到项目里的 `backend/.env`，
所以在 `backend/`、`backend/cmd/server/` 或仓库根目录下启动都能读到同一份配置。
要指定别的位置，用环境变量 `AJA_ENV_FILE` 指向 `.env` 即可。

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
| `go run ./cmd/skillcheck CZ3101 2026-09-13` | 跑一次航班技能（EOOB 状态接口），打印观测值与问题 |
| `go run ./cmd/mockserver` | 前端联调用的假后端 |

## 已知限制

- **航班动态只走 EOOB 状态接口**，旅客链路里不再有航司官网和无头浏览器这一环：
  EOOB 的 Cloudflare 对 headless 一律拦截，航司官网配方同样被挡（南航"按航班号"
  点不到、东航等待超时），留着只会把"查询失败"写进旅客看到的依据里。
- 一次航班查询约 5~10 秒（EOOB HTTP 取数 + 模型生成建议），不追求低延迟。
- 路程时间（`etaMin`）是按直线距离×1.3÷30km/h 的**估算值**（非实时导航，`traffic` 保持 null）；机场内步行耗时没有数据源，字段留空。

详细实施顺序见 [projectplan.md](./projectplan.md)。
