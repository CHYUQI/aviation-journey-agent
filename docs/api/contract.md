# API Contract v1

基于Agent的民航旅客智能出行系统 · 前后端接口契约

| 项 | 值 |
|---|---|
| 版本 | v1.0.0 |
| 状态 | 冻结 |
| 机器可读版本 | `docs/api/openapi.yaml` |
| 冲突处理 | 与 `openapi.yaml` 不一致时，以 `openapi.yaml` 为准 |
| 维护人 | 后端基础设施负责人（契约守门人） |

前端不读后端代码、不看数据库、不猜字段；后端不改字段名、不改语义、不擅自改枚举。
所有变更走第 11 节流程。

---

## 1. 通用约定

### 1.1 服务地址

| 环境 | 地址 | 说明 |
|---|---|---|
| 真实链路 | `http://localhost:8080` | Gin + DataAgent + AdviceAgent |
| 联调 mock | `http://localhost:8081` | 固定夹具，不调用模型与外部数据源 |
| 前端开发 | `http://localhost:5173` | Vite 代理 `/api` 到上面之一 |

统一前缀 `/api/v1`。

### 1.2 命名与格式

- 字段名一律 camelCase，数组字段用复数。
- 请求与响应均为 `application/json; charset=utf-8`，SSE 除外。
- 行程 ID 是不透明字符串，客户端不得解析其结构。

### 1.3 时间与单位

| 类型 | 约定 | 示例 |
|---|---|---|
| 时间点 | RFC3339，必须带时区 | `2026-09-11T15:00:00+08:00` |
| 日期 | `YYYY-MM-DD` | `2026-09-11` |
| 时长 | 整数分钟，字段名以 `Min` 结尾 | `etaMin: 52` |

禁止使用 Unix 时间戳、禁止使用不带时区的时间字符串。
唯一例外是 `advice.cards[].value`——它是**已经格式化好的展示字符串**，不是机读数据。

### 1.4 时间由服务端决定

客户端**不上报当前时间**，理由：

1. 设备时间可能不准、时区设置可能错误，用它算"还剩多少缓冲"会直接导致误判。
2. HTTP 请求到达时刻本身就是当前时间，服务端用 `time.Now()` 即可。
3. 全局只有一个时钟，差值计算才不会跳。

前端需要做倒计时时，用 `state.updatedAt`（服务端生成的快照时间）校正本地时钟偏移：

```typescript
const offset = new Date(snapshot.state.updatedAt).getTime() - Date.now()
const remainMs = new Date(node.time).getTime() - (Date.now() + offset)
```

### 1.5 空值语义

```
null      已查询，但没有可用数据
具体值     有依据的取值
```

四条硬性规则：

1. **状态字段始终输出**，禁止用"省略字段"表示未知。
2. 未知一律用 `null`，不得用 `0`、`""` 伪装成有效数据。
3. 数组永远返回数组，无内容时返回 `[]`，不返回 `null`。
4. 后端 Go 结构体的可空数值/时间字段必须用指针并**去掉 `omitempty`**。
   否则 `etaMin: 0` 会被序列化吞掉，前端无法区分"0 分钟"和"没有数据"。

---

## 2. 接口总览

| # | 方法 | 路径 | 用途 | 成功码 |
|---|---|---|---|---|
| 1 | POST | `/api/v1/journey` | 创建行程并触发分析 | 202 |
| 2 | POST | `/api/v1/journey/{id}/location` | 上报定位或手动确认阶段 | 202 |
| 3 | GET | `/api/v1/journey/{id}/state` | 查询当前快照 | 200 |
| 4 | GET | `/api/v1/journey/{id}/stream` | SSE 订阅快照 | 200 |

前端只需要这 4 个接口。航班、机场、路线、大模型全部在服务端内部完成，
前端不得直接调用任何外部数据源。

---

## 3. 接口详情

### 3.1 创建行程

```http
POST /api/v1/journey
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| flights | Flight[] | 是 | 至少 1 个，按行程顺序；v1 只处理第一个航段 |
| flights[].number | string | 是 | 航班号，大写不含空格 |
| flights[].date | string(date) | 是 | 出行日期 |
| flights[].from | string | 是 | 出发机场 IATA 三字码 |
| flights[].to | string | 是 | 到达机场 IATA 三字码 |
| hasBaggage | boolean | 是 | 是否携带托运行李 |

```json
{
  "flights": [
    { "number": "CA1234", "date": "2026-09-11", "from": "PVG", "to": "PEK" }
  ],
  "hasBaggage": true
}
```

响应 202：

```json
{ "journeyId": "j_7f3a91", "status": "processing" }
```

> 为什么是 202：分析要调用大模型和多个数据技能，可能耗时 5–30 秒。
> 后端不阻塞等待，客户端拿到 `journeyId` 后立即订阅 SSE 或轮询 `/state`。

错误：400、500。

---

### 3.2 上报定位或手动确认阶段

```http
POST /api/v1/journey/{id}/location
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| lat | number \| null | 否 | 纬度，与 `lng` 必须成对出现 |
| lng | number \| null | 否 | 经度 |
| stage | ManualStage \| null | 否 | 手动确认的当前阶段 |

**校验规则**

1. 至少提供 `lat` + `lng` 或 `stage` 之一，否则 400。
2. `lat` 与 `lng` 必须成对出现，只给一个视为 400。

**三种用法**

```json
// 1. 上报定位坐标
{ "lat": 31.2304, "lng": 121.4737 }

// 2. 手动确认阶段（定位不可用时）
{ "stage": "check_in" }

// 3. 坐标 + 阶段一起上报
{ "lat": 31.2304, "lng": 121.4737, "stage": "security" }
```

`ManualStage` 取值：`en_route` `at_airport` `check_in` `security` `waiting` `boarding`。

**行为规则**

| 情况 | 后端行为 |
|---|---|
| 只上报坐标 | 覆盖旅客位置，**清除此前手动确认的阶段**，以定位为准 |
| 只确认阶段 | 位置保持不变，阶段固定为旅客确认值 |
| 两者都传 | 坐标生效，且阶段固定为旅客确认值 |
| 定位失败 | 客户端不上报坐标，可改为上报 `stage`，或什么都不做 |

手动确认的阶段会一直生效，直到下一次上报坐标。
后端必须在 `advice.reasons` 中写明当前阶段来自旅客手动确认。

**两种方式都会触发重算，重算完成后立即通过 SSE 推送新快照，不得等到下一个 15 秒周期。**

响应 202：

```json
{ "journeyId": "j_7f3a91", "status": "processing" }
```

错误：400、404。

---

### 3.3 查询当前快照

```http
GET /api/v1/journey/{id}/state
```

前端的**唯一状态来源**，结构与 SSE 推送的 `data` 完全一致。

**status = ready**

```json
{
  "status": "ready",
  "journey": {
    "id": "j_7f3a91",
    "flights": [
      { "number": "CA1234", "date": "2026-09-11", "from": "PVG", "to": "PEK" }
    ],
    "hasBaggage": true
  },
  "state": {
    "flightStatus": "on_time",
    "gate": "B27",
    "timeline": [
      { "label": "开始登机", "time": "2026-09-11T14:20:00+08:00" },
      { "label": "登机口关闭", "time": "2026-09-11T14:45:00+08:00" },
      { "label": "起飞", "time": "2026-09-11T15:00:00+08:00" }
    ],
    "etaMin": 52,
    "traffic": "heavy",
    "guide": [
      "T2 入口 → 值机柜台，约 4 分钟",
      "值机柜台 → 安检，约 6 分钟",
      "安检 → B27 登机口，约 14 分钟"
    ],
    "quality": "degraded",
    "updatedAt": "2026-09-11T12:36:00+08:00"
  },
  "advice": {
    "stage": "en_route",
    "risk": "yellow",
    "alert": "安检排队上升到 45 分钟，缓冲可能不足",
    "cards": [
      { "label": "预计到达机场", "value": "52 分钟" },
      { "label": "安检排队", "value": "18 分钟" },
      { "label": "剩余缓冲", "value": "18 分钟" },
      { "label": "最晚出发", "value": "13:28" }
    ],
    "actions": [
      {
        "id": "leave_now",
        "title": "尽快出发",
        "detail": "当前路况拥堵，预计 52 分钟到达机场",
        "nav": {
          "app": "amapuri://route/plan?dlat=31.1443&dlon=121.8083&dname=PVG%20T2",
          "web": "https://uri.amap.com/navigation?to=31.1443,121.8083,PVG%20T2&mode=car"
        }
      },
      {
        "id": "contact_airline",
        "title": "联系航空公司确认",
        "detail": "如果缓冲继续下降，建议提前咨询改签政策",
        "nav": null
      }
    ],
    "reasons": [
      "航班数据更新于 12:35，来源 flight_status",
      "安检排队为估计值，置信度低"
    ]
  }
}
```

**status = processing**：结构完全相同，未知字段为 `null`，数组为 `[]`。

```json
{
  "status": "processing",
  "journey": { "...": "同上" },
  "state": {
    "flightStatus": "unknown",
    "gate": null,
    "timeline": [],
    "etaMin": null,
    "traffic": null,
    "guide": [],
    "quality": "unknown",
    "updatedAt": "2026-09-11T08:30:01+08:00"
  },
  "advice": {
    "stage": "unknown",
    "risk": "unknown",
    "alert": null,
    "cards": [],
    "actions": [],
    "reasons": ["分析进行中，暂无建议"]
  }
}
```

**status = failed**：结构相同，多一行 `error`。

```json
{
  "status": "failed",
  "journey": { "...": "同上" },
  "state": { "...": "同上" },
  "advice": { "...": "同上，reasons 为 []" },
  "error": "行动决策 Agent 调用失败"
}
```

错误：404。

---

### 3.4 SSE 实时订阅

```http
GET /api/v1/journey/{id}/stream
Accept: text/event-stream
```

```
retry: 5000

event: snapshot
data: {"status":"processing","journey":{...},"state":{...},"advice":{...}}

event: heartbeat
data: {"at":"2026-09-11T12:36:15+08:00"}
```

| 时机 | 事件 | data |
|---|---|---|
| 连接建立后立即 | `snapshot` | 完整 JourneySnapshot |
| 快照发生变化 | `snapshot` | 完整 JourneySnapshot |
| 位置或阶段更新并重算完成 | `snapshot` | 完整 JourneySnapshot |
| 15 秒内无变化 | `heartbeat` | `{"at": "RFC3339"}` |
| 分析失败 | `snapshot` | 快照中 `status = "failed"` |

客户端要求：

1. 使用 `EventSource`，按 `retry` 自动重连。
2. 只监听 `snapshot` 事件；`heartbeat` 只用于刷新"实时连接正常"指示。
3. 连接失败或断开超过 20 秒，回退到 15 秒轮询 `/state`，并提示"实时连接已断开"。
4. 重连成功后用最新快照**整体覆盖**本地状态，不做增量合并。

错误：404。

---

## 4. 数据结构

响应路径上共 **9 个对象、36 个字段**，最大嵌套 3 层（`snapshot → advice → actions[] → nav`）。

```
JourneySnapshot
├── status          AnalysisStatus            必填
├── journey         Journey                   必填
│   ├── id             string
│   ├── flights        Flight[]
│   │   ├── number       string               航班号，大写
│   │   ├── date         string(date)         YYYY-MM-DD
│   │   ├── from         string               IATA 三字码
│   │   └── to           string               IATA 三字码
│   └── hasBaggage     boolean
├── state           State                     必填
│   ├── flightStatus    FlightStatus
│   ├── gate            string | null
│   ├── timeline        TimelineNode[]        按时间升序
│   │   ├── label        string               如"开始登机"
│   │   └── time         date-time            RFC3339，供倒计时
│   ├── etaMin          integer | null        到机场分钟数
│   ├── traffic         Traffic | null
│   ├── guide           string[]              机场内逐段文字指引
│   ├── quality         Quality
│   └── updatedAt       date-time
├── advice          Advice                    必填
│   ├── stage           Stage
│   ├── risk            Risk
│   ├── alert           string | null         变了就弹的那句话
│   ├── cards           Card[]                所有展示数字
│   │   ├── label        string
│   │   └── value        string               已格式化，如 "18 分钟"
│   ├── actions         Action[]              已排序
│   │   ├── id           string               如 leave_now
│   │   ├── title        string
│   │   ├── detail       string
│   │   └── nav          Nav | null           有值才显示导航按钮
│   │       ├── app      string               应用内跳转，优先
│   │       └── web      string               网页兜底
│   └── reasons         string[]              依据，含来源与时间
└── error           string | null             仅 status=failed
```

**请求侧**

```
CreateJourneyRequest
├── flights         Flight[]        必填，至少 1 个
└── hasBaggage      boolean         必填

UpdateLocationRequest
├── lat             number | null   可选，与 lng 成对
├── lng             number | null   可选
└── stage           ManualStage | null  可选，手动确认阶段

AnalysisAccepted
├── journeyId       string
└── status          AnalysisStatus
```

**字段约定**

| 字段 | 约定 |
|---|---|
| 所有 `| null` 字段 | 始终输出，未知为 `null` |
| 所有数组 | 无内容返回 `[]` |
| 所有时间 | RFC3339 带时区 |
| 所有 `*Min` | 整数分钟 |
| `state` | 放状态与时间，只包含有依据的数据 |
| `advice.cards[]` | 放所有面向旅客的数字，前端数字只从这一处取 |
| `advice.stage` | 可能是自动判断，也可能是旅客手动确认 |
| `advice.alert` | 后端决定是否需要提醒；前端只判断"和上次不一样就弹" |
| `advice.actions[]` | 已按优先级排序，前端不再排序 |
| `error` | 一句话，前端直接展示，不做分支 |

---

## 5. 枚举

| 枚举 | 取值 |
|---|---|
| AnalysisStatus | `processing` `ready` `failed` |
| Stage | `unknown` `en_route` `at_airport` `check_in` `security` `waiting` `boarding` `departed` `disrupted` |
| ManualStage | `en_route` `at_airport` `check_in` `security` `waiting` `boarding` |
| Risk | `unknown` `green` `yellow` `orange` `red` |
| Quality | `complete` `degraded` `unknown` |
| Traffic | `unknown` `light` `moderate` `heavy` `severe` |
| FlightStatus | `unknown` `scheduled` `on_time` `delayed` `boarding` `departed` `cancelled` `diverted` |

`ManualStage` 是 `Stage` 的子集，去掉了旅客无法自行确认的 `unknown`、`departed`、`disrupted`。

开放字符串（前端原样展示，不做分支）：`state.guide[]`、`advice.reasons[]`、`advice.cards[].label`、`advice.cards[].value`、`error`。

**兜底规则**

- 不认识的 `risk` → 按 `unknown` 渲染（灰色 + "数据不足"）
- 不认识的 `flightStatus` → 原样显示字符串，不加颜色
- 不认识的 `stage` → 时间轴不高亮任何节点

---

## 6. 错误处理

HTTP 层错误统一返回一行：

```json
{ "error": "journey not found" }
```

| 状态码 | 含义 | 前端处理 |
|---|---|---|
| 400 | 请求参数不合法 | 表单标红并提示 |
| 404 | 行程不存在 | 回到创建行程页 |
| 500 | 服务端错误 | 通用错误提示 |
| 502 / 504 | 模型调用失败或超时 | 提示重试 |

分析过程中的失败**不通过 HTTP 返回**，而是体现在快照里：
`status = "failed"` + `error` 一句话，前端展示横幅与"重新分析"入口。

---

## 7. 前端渲染约定

### 7.1 数据来源映射

| 组件 | 数据来源 |
|---|---|
| StatusBar | `status` + `state.flightStatus` + `advice.stage` + `advice.risk` |
| AlertBanner | `advice.alert` |
| MetricsRow | `advice.cards[]` ← 唯一的数字来源 |
| Timeline | `state.timeline[]` + `advice.stage` |
| AirportGuide | `state.guide[]` |
| ActionCardList | `advice.actions[]`，`nav != null` 时显示导航按钮 |
| FreshnessBar | `state.updatedAt` + `state.quality` |
| StageConfirm | 手动确认阶段的入口，调用 `POST /journey/{id}/location` |

### 7.2 三态

| status | 界面要求 |
|---|---|
| `processing` | 骨架屏；显示"正在分析行程"；不展示风险颜色；不渲染行动卡片 |
| `ready` | 正常渲染全部组件 |
| `failed` | 保留已有内容；横幅显示 `error` 与"重新分析"入口 |

### 7.3 提醒规则

`advice.alert` 是后端决定的"当前最该提醒旅客的一句话"，前端不做业务判断：

```typescript
if (snap.advice.alert && snap.advice.alert !== lastAlert) {
  toast(snap.advice.alert, snap.advice.risk === 'red' ? 'error' : 'warning')
}
lastAlert = snap.advice.alert
```

去重只需记住上一次的字符串；`null` 表示当前无需打扰旅客。

### 7.4 风险色

`green` 绿 · `yellow` 黄 · `orange` 橙 · `red` 红 · `unknown` 灰

**关键数据缺失时后端返回 `unknown`，前端绝不允许把 `unknown` 渲染成绿色。**

### 7.5 数据质量提示

`state.quality != complete` 时，必须展示"数据可能不完整"的提示，
并把 `advice.reasons[]` 中与数据相关的说明一并展示。

### 7.6 手动确认阶段

- 入口建议放在时间轴或状态条附近，选项固定为 6 个 `ManualStage`。
- 点击后调用 `POST /journey/{id}/location`，body 为 `{ "stage": "check_in" }`。
- 上报后界面进入 `processing`，收到新快照后状态条显示确认后的阶段。
- 定位恢复后前端继续正常上报坐标，后端会自动清除手动阶段，前端无需额外处理。
- 阶段是否为手动确认，由 `advice.reasons` 中的说明体现。

### 7.7 导航按钮

- 只有 `action.nav != null` 时才渲染"立即导航"按钮。
- 点击后优先打开 `action.nav.app`；失败或从桌面浏览器访问时使用 `action.nav.web`。
- 前端不内嵌地图 SDK、不渲染路线图、不采集定位轨迹。
- 机场内部只展示 `state.guide[]` 与时间轴。

---

## 8. 结构设计约束（防止再次膨胀）

这版结构刻意保持扁平，新增字段前先满足以下条件：

1. **每个字段必须对应界面上的一个可见位置**，对应不上就不加。
2. **不再新增纯容器对象**。只有一个字段的嵌套对象一律摊平到上一级。
3. **不再新增逐字段溯源结构**。数据来源写进 `advice.reasons[]`，
   数据新鲜度用 `state.quality` + `state.updatedAt`。
4. **派生值不单独建层**。计算结果显示在 `advice.cards[]` 里即可。
5. **提醒不是事件系统**。`advice.alert` 是一句话，不做 id、等级、历史列表。

已知取舍：当前结构无法逐字段展示"这个登机口来自哪个数据源、置信度多少"。
如果后续需要这种粒度，单独讨论，不在这版契约内。

---

## 9. 后端实现约束

1. 关键数据缺失时 `advice.risk` 必须为 `unknown` 或至少 `yellow`，**不得返回 `green`**。
2. 禁止用无依据的默认值填充字段；确需保守假设时必须在 `advice.reasons` 中说明。
3. 单个数据源失败不得导致整个分析失败：其余字段照常返回，问题写入 `state.quality` 与 `advice.reasons`。
4. AdviceAgent 只消费 WorldState，不直接访问外部数据源。
5. 后端不执行改签、支付、叫车；`action` 只是展示型动作。
6. 同一行程的重算必须串行，避免并发覆盖快照。
7. 时间一律取服务端时钟，不采用客户端上报的时间。

---

## 10. 本地联调

```bash
cd backend && go run ./cmd/server          # 真实链路 :8080
cd backend && go run ./cmd/mockserver      # 固定夹具 :8081
cd frontend && npm run dev                 # :5173
```

mock 用航班号切换场景：

| 航班号 | 场景 | 结果 |
|---|---|---|
| 任意（如 CA1234） | 正常 | `ready`，`yellow`，带导航行动 |
| MU9999 | 延误 + 登机口变更 | `ready`，`orange`，额外"联系航司"行动 |
| FAIL | 模型失败 | `failed`，带 error 文案 |

mock 先返回 `processing`，2 秒后切换为 `ready` / `failed`，用于验证加载态与 SSE 推送。
mock 同样支持 `{ "stage": "..." }` 的手动确认，收到后会把 `advice.stage` 改为确认值。

---

## 11. 变更规则

| 变更类型 | 是否破坏兼容 | 处理方式 |
|---|---|---|
| 新增字段、新增 card、新增 action | 否 | 直接加，前端保证兜底 |
| 新增枚举值 | 否（前端有兜底） | 增加后通知前端 |
| 改字段名 / 改类型 / 改语义 / 删字段 | **是** | 同步改 openapi.yaml 并通知全员 |
| 改路径 / 改状态码 / 改事件名 | **是** | 同上 |

流程：**先改 `openapi.yaml` → 提交 PR 并通知前端 → 再改代码**。
契约由契约守门人统一维护。

---

## 12. 前端类型生成

```bash
cd frontend
npm i -D openapi-typescript   # 首次
npm run gen:api               # 生成 src/types/api.d.ts
```

```typescript
import type { components } from '../types/api'

export type JourneySnapshot = components['schemas']['JourneySnapshot']
export type Advice = components['schemas']['Advice']
export type Action = components['schemas']['Action']
```

生成文件为自动产物，不要手动编辑。契约变更后重新生成，类型不匹配会在 `tsc` 阶段报错。

---

## 13. 未决事项

| # | 事项 | 当前处理 |
|---|---|---|
| 1 | 多航段：`flights[]` 是数组，但 `state` 描述单航段 | v1 只处理第一个航段 |
| 2 | 逐字段数据溯源（来源 / 置信度 / 有效期） | 用 `advice.reasons[]` 近似表达 |
| 3 | 页面关闭后的系统级推送 | v1 只做页面内提示 + 浏览器 Notification |
| 4 | 路由用单数 `/journey` 与 UML 顺序图保持一致 | 改为 `/journeys` 属于破坏性变更 |
| 5 | 手动确认阶段是否需要一个"取消确认"接口 | 当前由上报坐标自动解除 |

