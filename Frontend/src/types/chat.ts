export type ChatRole = "user" | "assistant"

export interface ChatMessage {
  id: string
  chatId: string
  role: ChatRole
  content: string
  createdAt: number
  /** Set on an optimistic message that the server has not confirmed yet. */
  pending?: boolean
}

export interface Chat {
  id: string
  title: string
  provider: string
  model: string
  createdAt: number
  updatedAt: number
}

export interface ChatDetail extends Chat {
  messages: ChatMessage[]
}

export interface ModelInfo {
  id: string
  label: string
  provider: string
}

export interface ProviderInfo {
  id: string
  name: string
  defaultModel: string
  models: ModelInfo[]
}

export interface ModelSelection {
  provider: string
  model: string
}

export interface Usage {
  inputTokens: number
  outputTokens: number
  model: string
}
