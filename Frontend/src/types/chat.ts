export type ChatRole = "user" | "assistant"

export interface ChatMessage {
  id: string
  chatId: string
  role: ChatRole
  content: string
  /** The model's thinking text, when the provider exposes it. */
  reasoning?: string
  /** How long the model spent thinking, in milliseconds. */
  thinkingMs?: number
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

/** A provider row on the Providers page. */
export interface ProviderRow {
  id: string
  name: string
  /** The OpenAI-compatible endpoint, for a provider the user added. */
  baseUrl: string
  /**
   * The effective credential: the stored override, or the value from the
   * backend's environment. Only ever sent to this local API.
   */
  apiKey: string
  /** Credentials are configured for this provider. */
  connected: boolean
  /** Whether the provider may be used; turning it off hides its models. */
  enabled: boolean
  /** The user added this provider, so it can also be renamed or removed. */
  custom: boolean
  defaultModel: string
  /** Chat models the live catalog exposes for this provider. */
  modelCount: number
}

/** The fields the providers page can write. */
export interface ProviderInput {
  name?: string
  baseUrl?: string
  apiKey?: string
  enabled?: boolean
}

export interface Usage {
  inputTokens: number
  outputTokens: number
  model: string
}
