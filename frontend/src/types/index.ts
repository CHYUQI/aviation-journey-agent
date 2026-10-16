// 统一从 docs/api/openapi.yaml 生成的类型派生别名。
//
// 这里只做别名，不手写字段。契约变更后运行：npm run gen:api
// 字段定义以 docs/api/openapi.yaml 为准。

import type { components } from './api'

type Schemas = components['schemas']

// ---- 快照 ----
export type JourneySnapshot = Schemas['JourneySnapshot']
export type Journey = Schemas['Journey']
export type Flight = Schemas['Flight']
export type State = Schemas['State']
export type TimelineNode = Schemas['TimelineNode']
export type Advice = Schemas['Advice']
export type Card = Schemas['Card']
export type Action = Schemas['Action']
export type Nav = Schemas['Nav']

// ---- 枚举 ----
export type AnalysisStatus = Schemas['AnalysisStatus']
export type Stage = Schemas['Stage']
export type ManualStage = Schemas['ManualStage']
export type Risk = Schemas['Risk']
export type Quality = Schemas['Quality']
export type Traffic = Schemas['Traffic']
export type FlightStatus = Schemas['FlightStatus']

// ---- 请求与响应 ----
export type CreateJourneyRequest = Schemas['CreateJourneyRequest']
export type UpdateLocationRequest = Schemas['UpdateLocationRequest']
export type AnalysisAccepted = Schemas['AnalysisAccepted']
export type ApiErrorResponse = Schemas['Error']

// ---- 枚举中文标签（纯展示映射，不参与业务判断） ----
export const AnalysisStatusLabel: Record<AnalysisStatus, string> = {
  processing: '分析中',
  ready: '已就绪',
  failed: '分析失败',
}

export const StageLabel: Record<Stage, string> = {
  unknown: '未知',
  en_route: '前往机场',
  at_airport: '已到机场',
  check_in: '办理值机',
  security: '安检中',
  waiting: '候机中',
  boarding: '登机中',
  departed: '已起飞',
  disrupted: '行程中断',
}

export const RiskLabel: Record<Risk, string> = {
  unknown: '未知',
  green: '低风险',
  yellow: '需关注',
  orange: '较高风险',
  red: '高风险',
}
