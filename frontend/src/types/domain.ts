export interface AdviceMetric { key: string; label: string; value: string | number; unit: string }
export interface Navigation { provider: string; uri: string; fallbackUrl?: string }
export interface AdviceAction { id: string; kind: string; title: string; detail: string; priority: number; navigation?: Navigation }
export interface Advice { stage: string; riskLevel: string; riskScore: number; metrics: AdviceMetric[]; actions: AdviceAction[]; reasoning: string[] }
export interface JourneySnapshot {
  journey: Record<string, unknown>
  state: { dataQuality?: { status: string; issues?: string[] }; travel?: { etaMin?: number; traffic?: string }; airport?: { code?: string; terminal?: string }; [key: string]: unknown }
  advice: Advice
  generatedAt: string
}
