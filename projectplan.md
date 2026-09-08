# 航旅智行项目计划

- 项目名称：航旅智行（Aviation Journey Agent）
- 文档日期：2026-09-07
- 当前目标：在单 Go 二进制和单页前端约束下，完成“真实数据处理 Agent → 行动决策 Agent → 仪表盘/SSE”的可运行系统。

## 1. 项目定位

系统面向民航旅客起飞前的行程规划和延误风险预警。系统把三类状态融合起来：

1. 航班状态：计划起飞、预计起飞、登机时间、登机口关闭时间、登机口、航班异常状态。
2. 机场状态：航站楼、值机/托运、安检等待、机场内部步行节点和登机口指引。
3. 旅客状态：当前位置、是否携带托运行李、预计到达机场时间、当前旅程阶段。

系统输出“下一步行动建议”，不自动执行改签、支付、订票或其他高影响操作。

## 2. 最终架构决策

### 2.1 部署形态

```text
Vue3 + TypeScript SPA
        │ HTTP / SSE
        ▼
单个 Go 二进制
├── Gin API
├── Journey Runtime
├── DataAgent
├── AdviceAgent
├── Skill Registry
├── Memory Store
└── SSE Publisher
```

开发环境可以前后端分开运行；演示或部署环境使用 `go:embed` 将 `frontend/dist` 嵌入 Go 二进制。

### 2.2 两个 Agent

#### DataAgent：数据处理 Agent

职责：

- 检查字段是否缺失、过期或互相冲突。
- 选择并调用 Skill 查询真实数据。
- 将不同来源的数据标准化。
- 记录每个关键字段的来源、观测时间、有效期和置信度。
- 只进行有明确依据的派生计算。
- 输出 `WorldState`、数据质量和数据问题。

禁止：

- 直接生成“立即出发”“去安检”等行动建议。
- 直接操作前端、SSE 或 Store。
- 用无来源的默认值伪装成真实数据。

#### AdviceAgent：行动决策 Agent

职责：

- 根据 `WorldState` 推导旅客阶段。
- 执行登机、托运、值机、安检、机场内部步行等硬约束。
- 计算剩余缓冲和风险等级。
- 生成、排序和解释行动建议。
- 输出前端可以直接渲染的 `Advice`。

禁止：

- 调用外部航班、机场或地图 API。
- 修改外部数据。
- 自动执行改签、支付或叫车。

### 2.3 Journey Runtime

`Journey Runtime` 是两个 Agent 之间的编排层，负责：

- 创建行程并触发首次分析。
- 在位置更新后立即重算。
- 按固定周期刷新活跃行程。
- 防止同一行程的计算并发冲突。
- 保存最新 `JourneySnapshot`。
- 向 SSE 连接发布最新快照。

SSE 只负责发送结果，不负责执行业务分析。

## 3. 数据处理原则

### 3.1 真实数据优先

运行时不接入模拟航班、模拟机场或模拟排队数据。DataAgent 通过 Skill Registry 调用真实数据来源，包括：

- 航司或航班动态接口。
- 机场或航站楼公开数据。
- 路径规划/交通 ETA 接口。
- 官方网站或公开信息检索能力。

测试时可以使用 fixture，但 fixture 不能进入正式运行链路。

### 3.2 数据分层

每个状态字段需要区分：

- `observed`：外部真实观测值。
- `derived`：由真实观测值和规则计算得到的派生值。
- `stale`：曾经获取但已经超过有效期的数据。
- `unknown`：当前没有足够证据的数据。

不能把未知值直接写成一个看似精确的默认数字。

### 3.3 Evidence

每个关键字段建议记录：

```text
field       字段路径，例如 flight.gate
source      数据来源
observedAt  观测时间
expiresAt   有效期
confidence  high / medium / low
method      api / search / derived / previous
```

AdviceAgent 必须把关键的低置信度和过期信息放进 `reasoning`。

### 3.4 风险安全规则

当以下关键数据缺失时，不能输出“绿色且安全”：

- 登机口关闭时间。
- 航班当前状态。
- 到机场 ETA。
- 机场内部耗时。
- 当前定位或定位新鲜度。

可以使用 `riskLevel=unknown`，或者至少使用黄色风险并标注 `dataQuality=degraded`。
## 4. 前后端目录

```text
aviation-journey-agent/
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/domain/
│   ├── internal/runtime/
│   ├── internal/agent/data/
│   ├── internal/agent/advice/
│   ├── internal/skill/
│   ├── internal/api/
│   └── internal/store/
│
├── frontend/
│   ├── src/api/
│   ├── src/components/
│   ├── src/stores/
│   ├── src/types/
│   ├── src/views/
│   └── src/assets/
│
├── docs/
├── README.md
└── projectplan.md
```

前端不再包含 `RouteMap.vue`，只保留：

- 状态条。
- 数字指标卡。
- 行程时间轴。
- 行动卡片。
- 高德导航按钮。
- 数据来源与新鲜度提示。

后端仍然保留 `RouteETASkill`，因为它负责计算出发地到机场的 ETA，而不是渲染地图。

## 5. 主要接口

### 创建行程

```http
POST /api/v1/journey
Content-Type: application/json
```

请求：

```json
{
  "flights": [
    {
      "number": "CA1234",
      "date": "2026-09-10",
      "from": "PVG",
      "to": "PEK"
    }
  ],
  "passenger": {
    "hasBaggage": true,
    "walkSpeed": 1.2
  }
}
```

创建后触发一次 DataAgent → AdviceAgent 计算，并返回完整的 `JourneySnapshot`。

### 更新位置

```http
POST /api/v1/journey/:id/location
```

位置更新后立即重算，不等待下一个 15 秒周期。

### 查询当前快照

```http
GET /api/v1/journey/:id/state
```

这是前端的单一状态来源，返回：

```json
{
  "journey": {},
  "state": {},
  "advice": {},
  "generatedAt": "..."
}
```

### SSE 实时流

```http
GET /api/v1/journey/:id/stream
```

服务端每 15 秒检查并发布最新快照。事件名称为 `snapshot`，事件数据结构与 `/state` 相同。SSE 连接断开时，前端回退到状态查询或重新连接。

## 6. 关键决策规则

### 未到机场

```text
buffer = latestDeparture - 当前时间 - 路程 ETA
```

建议逻辑：

- `buffer >= 30 分钟`：按计划出发或保持当前计划。
- `0 <= buffer < 30 分钟`：尽快出发。
- `buffer < 0`：高风险，建议立即出发并提示联系航司。
- 关键数据未知：不能返回绿色低风险。

### 已到机场

根据以下因素判断：

- 值机截止时间。
- 托运截止时间。
- 当前排队时间。
- 安检时间。
- 到登机口的步行时间。
- 登机时间和登机口关闭时间。

机场内部只输出时间轴和文字指引，例如：

```text
入口 → 值机柜台 → 安检 → B27 登机口
预计机场内部耗时：18 分钟
```

### 航班异常

航班取消、大幅延误、登机口变化等情况进入 `disrupted` 分支，系统只提供：

- 当前异常说明。
- 数据更新时间和来源。
- 联系航司或前往服务柜台的建议。

系统不自动执行改签。

## 7. 行动卡片设计

行动卡片是展示型动作，不是后端命令执行器：

```json
{
  "id": "leave_now",
  "kind": "navigate",
  "title": "立即前往机场",
  "detail": "当前道路预计耗时 42 分钟，建议现在出发",
  "priority": 1,
  "navigation": {
    "provider": "amap",
    "uri": "amapuri://route/plan",
    "fallbackUrl": "https://uri.amap.com/navigation"
  }
}
```

前端点击导航按钮后跳转高德，不嵌入任何地图 SDK。
## 8. 开发顺序

### 阶段一：领域模型与运行时骨架

- 完成 `Journey`、`WorldState`、`Advice`、`JourneySnapshot`。
- 完成内存 Store。
- 完成 Journey Runtime 的创建、查询、重算流程。
- 确认两个 Agent 的接口边界。

### 阶段二：DataAgent 与 Skill Registry

- 完成 Skill 接口和注册表。
- 实现航班状态 Skill。
- 实现机场状态 Skill。
- 实现路线 ETA Skill。
- 实现官方信息检索 Skill。
- 完成来源、时间戳、有效期和置信度记录。
- 完成缺失、过期和冲突字段处理。

### 阶段三：AdviceAgent

- 完成旅客阶段推导。
- 完成截止时间和机场内部耗时规则。
- 完成缓冲时间和风险等级。
- 完成行动生成和优先级排序。
- 完成 reasoning 可解释信息。
- 验证关键数据未知时不会错误输出绿色风险。

### 阶段四：REST 与 SSE

- 完成创建行程接口。
- 完成位置更新接口。
- 完成状态查询接口。
- 完成 SSE 首次快照、定期快照和断线处理。
- 将 SSE 推送数据和 `/state` 数据统一为同一结构。

### 阶段五：Vue 仪表盘

- 完成状态条和风险颜色。
- 完成指标卡。
- 完成行程时间轴。
- 完成行动卡片。
- 完成高德导航按钮。
- 完成数据新鲜度和降级状态提示。
- 完成浏览器定位权限和定位失败提示。

### 阶段六：联调与打包

- 使用真实数据源完成一次完整闭环。
- 验证航班状态变化后能够重新计算。
- 验证位置变化后能够立即重算。
- 验证 SSE 和轮询回退。
- 验证数据源异常时不会让整个行程计算失败。
- 构建 `frontend/dist` 并接入 `go:embed`。

## 9. 验收标准

### 架构验收

- 两个 Agent 的职责没有交叉。
- AdviceAgent 不访问外部数据。
- DataAgent 不产生最终行动建议。
- API、SSE 和 Agent 不直接共享可变状态。
- 同一行程不会被并发重复计算。

### 数据验收

- 每个关键数据字段具有来源和时间信息。
- 缺失数据不会被无标记地伪造成真实数据。
- 数据获取失败可以返回部分 `WorldState` 和可解释问题。
- 过期数据会影响数据质量标识。

### 功能验收

- 可以创建行程。
- 可以更新旅客位置。
- 可以获取当前快照。
- 可以通过 SSE 获得最新快照。
- 可以显示航班、机场、旅客和路线相关指标。
- 可以展示下一步行动。
- 未到机场时可以跳转高德导航。
- 机场内部可以通过时间轴和文字指引表达。
- 不会自动改签、支付或叫车。

## 10. 当前明确不加入的内容

- Kafka、RabbitMQ 等消息队列。
- Redis 或独立数据库。
- WebSocket。
- 独立模型服务。
- 前端地图 SDK。
- 嵌入式地图展示。
- 自动改签、自动支付、自动叫车。
- 将模拟数据作为正式运行时数据源。
- 没有证据时硬编码看似精确的机场排队时间。

## 11. 后续扩展点

如果课程展示完成后需要扩展，可以在不破坏当前架构的前提下加入：

- 多航段和中转衔接。
- 更丰富的官方数据 Skill。
- 持久化数据库。
- 模型辅助的自然语言解释。
- 独立的通知服务。
- 真实的航司业务操作，但需要新增明确的权限、确认和审计层。
