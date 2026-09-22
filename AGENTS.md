# AGENTS.md

本文件约束所有在本仓库工作的 AI 助手（Codex、Claude Code、Cursor 等）与开发者。
**违反以下规则的改动一律不接受。**

---

## 0. 项目一句话

面向民航旅客的起飞前行程规划与延误风险预警系统。
单个 Go 二进制承载 API + DataAgent + AdviceAgent + SSE，前端是 Vue3 + TypeScript 单页仪表盘。

**前端只渲染，后端只算数。**

---

## 1. 最高规则：契约是唯一真源

`docs/api/openapi.yaml` 是全项目唯一真源。前端不读后端代码，后端不读前端代码，两边都只读契约。

| 操作 | 是否允许 |
|---|---|
| 新增字段、新增 card、新增 action | 允许（前端必须已做兜底） |
| 新增枚举值 | 允许（前端必须已做兜底） |
| 改字段名 / 改类型 / 改语义 | **禁止**，必须走变更流程 |
| 删除字段 | **禁止**，必须走变更流程 |
| 改路径 / 改状态码 / 改 SSE 事件名 | **禁止**，必须走变更流程 |

### 契约变更流程（缺一步都不算完成）

```
1. 改 docs/api/openapi.yaml
2. cd frontend && npm run gen:api
3. 同步更新 docs/api/contract.md
4. 通知对方（前端 <-> 后端）
5. 再改代码
```

`docs/api/openapi.yaml` 只由**契约守门人**（后端基础设施负责人）修改，其他人提需求。

---

## 2. 改动范围

| 目录 / 文件 | 谁可以改 |
|---|---|
| `backend/` | 后端负责人 |
| `frontend/` | 前端负责人 |
| `docs/api/` | 契约守门人 |
| `docs/*.docx`、`projectplan.md` | 文档负责人 |
| `frontend/src/types/api.d.ts` | **任何人都禁止手改**（自动生成） |

跨范围改动必须先说一声，不要顺手改对方的目录。

---

## 3. 前端规则

### 3.1 类型

- 类型只能从 `src/types/index.ts` 引入。
- **禁止手写接口类型**，禁止在组件里重新定义 `State`、`Advice` 这类结构。
- **禁止用 `as any`、`@ts-ignore`、`@ts-expect-error` 绕过契约相关的类型错误。**
  类型报错说明契约和代码对不上，停下来找后端确认。
- 契约变更后重新生成类型：`npm run gen:api`。

### 3.2 只渲染，不判断

| 允许（展示逻辑） | 禁止（业务判断） |
|---|---|
| 按 `risk` 映射颜色 | 自己算"还来不来得及" |
| 按数组顺序渲染 `actions` | 自己给行动排序 |
| 格式化时间、拼 `xx 分钟` | 自己算缓冲时间 |
| 按 `status` 切换骨架屏 | 自己推断航班会不会延误 |
| 时间轴高亮下一个未到达节点 | 自己判断旅客在哪个阶段 |

数字全部来自 `advice.cards[]` 与 `state`，不要在组件里做算术。

### 3.3 三态

- `status === 'processing'` → 骨架屏，不显示风险颜色，不渲染行动卡片
- `status === 'ready'` → 正常渲染
- `status === 'failed'` → 展示 `error` + 重试入口

`risk === 'unknown'` **绝不允许渲染成绿色**。

### 3.4 提醒

`advice.alert` 是后端决定的"当前最该提醒旅客的一句话"：

```typescript
if (snap.advice.alert && snap.advice.alert !== lastAlert) {
  toast(snap.advice.alert, snap.advice.risk === 'red' ? 'error' : 'warning')
}
lastAlert = snap.advice.alert
```

不需要 alert id、不需要等级字段、不要自己写提醒规则。

### 3.5 导航

- 只有 `action.nav != null` 时才渲染导航按钮
- 优先打开 `nav.app`，失败回退 `nav.web`
- 不内嵌地图 SDK、不渲染路线图、不采集轨迹

---

## 4. 后端规则

### 4.1 返回值

- 响应必须能通过 `docs/api/openapi.yaml` 校验。
- 可空字段用指针（`*int` / `*string` / `*time.Time`）并**去掉 `omitempty`**。
  否则 `etaMin: 0` 会被序列化吞掉，前端分不清"0 分钟"和"没有数据"。
- 数组无内容时返回 `[]`，不能返回 `null`。
- 时间一律取**服务端时钟**，不使用客户端上报的时间。

### 4.2 数据

1. **禁止编造数据。** 没有依据的字段一律返回 `null`。
   确需保守假设时必须在 `advice.reasons` 里写明。
2. **关键数据缺失时 `advice.risk` 必须为 `unknown` 或至少 `yellow`，不得返回 `green`。**
3. 单个数据源失败不得导致整个分析失败：其余字段照常返回，
   问题写进 `state.quality` 与 `advice.reasons`。
4. AdviceAgent 只消费 `WorldState`，不直接访问外部数据源。
5. 时间统一 RFC3339 带时区；时长统一整数分钟。
6. 同一行程的重算必须串行，避免并发覆盖快照。
7. 位置或阶段更新后立即重算，并**立即通过 SSE 推送**，不等下一个周期。

### 4.3 行为边界

**后端不执行改签、支付、叫车。** `action` 只是给前端展示的动作，不是命令。

---

### 4.4 数据获取规则（血泪教训，务必遵守）

#### 只用 EOOB 聚合源

航班与机场数据**只从 EOOB 取**，全部是普通 HTTPS 请求，不带浏览器：

| 数据 | 来源 | 说明 |
|---|---|---|
| 航班状态 / 计划起飞 / 登机口 | `apil.eoob.com/api/flight/status/{航班号}.json` | 必须带 `Origin` + `Referer`，否则 403 |
| 航班号 → 最近日期和航线 | `apil.eoob.com/api/flightnumbers/search.json` | 只用于补全行程 |
| 机场概况 / 客流 / 延误档位 | `www.eoob.com.cn/{IATA}` 与 `/api/delaybox/{IATA}` | 页面 HTML + 内嵌 JSON |
| 当前位置到机场耗时 | 本地估算（直线距离 × 1.3 ÷ 30km/h） | 不调外部接口，必须标注为估算 |

#### 请求头必须自洽，否则一律 403

`eoob.com.cn` 前面挂着 Cloudflare，响应头用 `Accept-Ch` / `Critical-Ch` 点名索要
Sec-CH-UA 系列客户端提示。只声称自己是 Chrome 却不带这些提示，等于自曝是脚本 ——
实测（Go http 客户端，各 5 次）：只带 UA 时 5/5 被 managed challenge，
补齐提示后 5/5 直接拿到页面。

**UA 与 `Sec-CH-UA` 的版本必须同源**，只改一个就是这次事故的根因。版本号集中在
`internal/skill/airport_status.go` 的 `eoobChromeVersion`，并有单测兜底。

#### 不用浏览器、不抓航司官网、不用搜索引擎

- EOOB 的 Cloudflare 对 headless 一律下发 managed challenge：页面能打开，
  元素永远等不到（实测干等 40s）；航司官网配方同样不可用
  （南航"按航班号"点不到、东航等待超时、国航有滑块验证码）。
- 这些路径**已经从代码里删除**。不要再用"换个浏览器 / 换个 UA / 换个搜索引擎"去救，
  也不要引入 Playwright/Puppeteer 或视觉模型。查不到就返回 `null` + issue，由上层降级。
- 模型联网检索（`enable_search`）同样不作为数据源。

#### 不要问注定答不了的问题

| 问法 | 结果 |
|---|---|
| 这个航班今天延误了吗（搜索引擎） | 拿不到，模型会开始编 |
| 这个航班今天几点起飞、从哪到哪 | EOOB 状态接口有 |
| 现在安检排队多久 | 拿不到 |
| 机场客流 / 延误档位 | EOOB 机场页有（计划值、统计窗口，引用时必须保留限定词） |

问不到就如实返回 `null`，**不要退而求其次用别的数字顶替**。

#### 数据可信度

- 只有 EOOB 接口字段/页面原文里出现过的值才能采信：不翻译、不补全、不换算；
- 关键数据缺失时 `advice.risk` 必须是 `unknown` 或至少 `yellow`，不得 `green`；
- 单个来源失败不得让整条分析失败：其余字段照常返回，
  问题写进 `state.quality` 与 `advice.reasons`。

## 5. 技术选型已冻结，禁止引入

- 消息队列（Kafka / RabbitMQ）
- WebSocket
- 数据库、Redis、独立持久化服务
- 前端地图 SDK、前端图表库（ECharts 等）
- 独立模型服务
- 把模拟数据作为**运行时**数据源
- 自动改签、自动支付、自动叫车

---

## 6. 数据结构约束（防止再次膨胀）

新增字段前先满足：

1. **每个字段必须对应界面上的一个可见位置**，对应不上就不加。
2. **不加纯容器对象**，只有一个字段的嵌套对象一律摊平。
3. **不做逐字段溯源结构**：数据来源写进 `advice.reasons[]`，
   新鲜度用 `state.quality` + `state.updatedAt`。
4. **派生值不单独建层**，计算结果显示在 `advice.cards[]`。
5. **提醒不是事件系统**：`advice.alert` 是一句话，不做 id / 等级 / 历史列表。

`state` 放状态与时间，`advice.cards` 放所有面向旅客的数字，两边不重复。

---

## 7. 提交前必须执行

```powershell
# 后端
cd backend
go build ./...
go vet ./...
go test ./...

# 前端
cd frontend
npm run build          # vue-tsc + vite

# 契约（改过 openapi.yaml 才需要）
npm run gen:api        # 之后 git diff 必须为空，否则说明类型没提交
```

任何一条不过，不许提交。

---

## 8. 遇到冲突怎么办

| 情况 | 处理 |
|---|---|
| 契约和代码对不上 | **以契约为准**，改代码 |
| 契约里没写的东西 | **先问，不要自己发明接口或字段** |
| 不知道该加什么字段 | 停下来问，不要猜 |
| 想改字段名让代码更顺眼 | **禁止**。契约已经冻结，改名走变更流程 |
| 文档（02/03）和契约对不上 | 以契约为准，同时提一句文档要同步 |

不确定就停下来问，**猜错字段的代价远大于问一句**。

---

## 9. 快速参考

| 内容 | 位置 |
|---|---|
| 接口契约（机器可读） | `docs/api/openapi.yaml` |
| 接口契约（人读） | `docs/api/contract.md` |
| 前端类型别名 | `frontend/src/types/index.ts` |
| 生成类型 | `frontend/src/types/api.d.ts`（勿手改） |
| 生成命令 | `cd frontend && npm run gen:api` |
| 真后端 | `cd backend && go run ./cmd/server`（:8080） |
| 前端开发 | `cd frontend && npm ci && npm run dev`（:5173） |
| 项目计划 | `projectplan.md` |
