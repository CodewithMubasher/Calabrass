export type ChatRole = "user" | "assistant"

/** How one tool call is faring. */
export type TraceStatus = "pending" | "running" | "ok" | "error" | "skipped"

/** One element of a turn: a stretch of thinking, a stretch of speech, or one tool call. */
export interface TracePart {
  kind: "reasoning" | "text" | "tool"
  /** The content of a reasoning or text part. */
  text?: string
  /** Tool call fields, set when kind is "tool". */
  callId?: string
  name?: string
  /** The model's arguments for the call, as raw JSON. */
  input?: string
  status?: TraceStatus
  /** What the tool returned, and what went wrong instead. */
  output?: string
  error?: string
  /** How long the call took, in milliseconds. */
  durationMs?: number
}

/** One model call, plus the results of the tools that call asked for. */
export interface TraceStep {
  /** 1-based, matching the harness loop's step counter. */
  number: number
  finishReason?: string
  parts: TracePart[]
}

/** The ordered record of one assistant turn: what the model thought, said and ran. */
export interface TurnTrace {
  steps: TraceStep[]
}

export interface ChatMessage {
  id: string
  chatId: string
  role: ChatRole
  content: string
  /** The model's thinking text, when the provider exposes it. */
  reasoning?: string
  /** How long the model spent thinking, in milliseconds. */
  thinkingMs?: number
  /** Output tokens the provider reported for a finished reply. */
  outputTokens?: number
  /** How long the finished reply took to generate, in milliseconds. */
  durationMs?: number
  /** The ordered record of the turn, when it ran tools or thought out loud. */
  trace?: TurnTrace
  createdAt: number
  /** Set on an optimistic message that the server has not confirmed yet. */
  pending?: boolean
}

export interface Chat {
  id: string
  title: string
  provider: string
  model: string
  /** The workspace folder this chat works in. Empty means the default. */
  workspaceDir: string
  /** Pinned chats lead the sidebar list. */
  pinned: boolean
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

/**
 * A workspace row: one folder the agent may work in. Every workspace lives
 * inside the project's `workspace` directory, and the agent cannot leave it.
 */
export interface WorkspaceRow {
  id: string
  name: string
  description: string
  /** The folder name inside the workspaces directory. */
  dir: string
  /** The workspace the agent uses until a chat picks its own; cannot be removed. */
  builtin: boolean
  /** The folder itself, for showing the user where their files are. */
  path: string
  /** The folder was removed from disk by hand. */
  missing: boolean
  createdAt: number
  updatedAt: number
}

/** The fields the workspaces page can write. */
export interface WorkspaceInput {
  name: string
  description?: string
}

/** A connector row on the Connectors page: one MCP server. */
export interface ConnectorRow {
  id: string
  name: string
  command: string
  /** Whether the agent may use the connector. */
  enabled: boolean
  createdAt: number
  updatedAt: number
}

/** The fields the connectors page can write. */
export interface ConnectorInput {
  name: string
  command: string
}

export interface Usage {
  inputTokens: number
  outputTokens: number
  model: string
}
