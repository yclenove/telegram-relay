/** 与后端 /api/v2 返回结构对齐的轻量类型定义。 */

export type BotRow = {
  id: number
  name: string
  is_default: boolean
  is_enabled: boolean
  remark: string
}

export type RuleRow = {
  id: number
  name: string
  priority: number
  match_source: string
  match_level: string
  destination_id: number
}

export type EventRow = {
  id: number
  event_id: string
  source: string
  level: string
  status: string
  created_at: string
}

export type AuditRow = {
  id: number
  actor_user_id: number | null
  action: string
  object_type: string
  object_id: string
  detail: string
  created_at: string
}

export type LoginResult = {
  access_token: string
  permissions: string[]
}
