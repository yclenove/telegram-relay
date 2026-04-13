import { http } from './http'
import type { AuditRow, BotRow, EventRow, LoginResult, RuleRow } from './types'

/** 封装管理端 REST 调用，页面只关心业务方法名与类型。 */

export async function login(username: string, password: string): Promise<LoginResult> {
  const { data } = await http.post<LoginResult>('/api/v2/auth/login', { username, password })
  return data
}

export async function fetchDashboard(): Promise<Record<string, number>> {
  const { data } = await http.get<Record<string, number>>('/api/v2/dashboard')
  return data
}

export async function fetchBots(): Promise<BotRow[]> {
  const { data } = await http.get<BotRow[]>('/api/v2/bots')
  return data
}

export async function createBot(payload: {
  name: string
  bot_token: string
  remark: string
  is_default: boolean
}): Promise<unknown> {
  const { data } = await http.post('/api/v2/bots', payload)
  return data
}

export async function fetchRules(): Promise<RuleRow[]> {
  const { data } = await http.get<RuleRow[]>('/api/v2/rules')
  return data
}

export async function createRule(payload: {
  name: string
  priority: number
  match_source: string
  match_level: string
  destination_id: number
}): Promise<unknown> {
  const { data } = await http.post('/api/v2/rules', payload)
  return data
}

export async function fetchEvents(): Promise<EventRow[]> {
  const { data } = await http.get<EventRow[]>('/api/v2/events')
  return data
}

export async function fetchAudits(): Promise<AuditRow[]> {
  const { data } = await http.get<AuditRow[]>('/api/v2/audits')
  return data
}
