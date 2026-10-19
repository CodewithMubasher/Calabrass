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
  /** The MCP connector the tool came from, when it came from one. */
  server?: string
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

export type ChatMode = "build" | "plan"

export interface Chat {
  id: string
  title: string
  provider: string
  model: string
  /** The chat's operating mode: "build" (full tools) or "plan" (read-only). */
  mode: ChatMode
  /** The workspace folder this chat works in. Empty means the default. */
  workspaceDir: string
  /** The agent this chat is associated with. Empty means no agent. */
  agentId: string
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

/** Which provider and model to use, and optionally which agent. */
export interface ModelSelection {
  provider: string
  model: string
  agentId?: string
}

/** A provider row on the Providers page. */
export interface ProviderRow {
  id: string
  name: string
  /** The OpenAI-compatible endpoint, for a provider the user added. */
  baseUrl: string
  /**
   * The provider's credential. It lives with the provider row, and is only ever
   * sent to this local API.
   */
  apiKey: string
  /** An endpoint is configured for this provider. */
  connected: boolean
  /** Whether the provider may be used; turning it off hides its models. */
  enabled: boolean
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

/**
 * One commit of a workspace's repository, as the Git & GitHub card shows it.
 */
export interface WorkspaceCommit {
  hash: string
  message: string
  /** The commit time, RFC 3339. */
  date: string
}

/**
 * A workspace folder's own git state, read from the folder on disk. `isRepo` is
 * false for a folder that is not under version control, which is a state the
 * card shows rather than an error.
 */
export interface WorkspaceGitInfo {
  isRepo: boolean
  /** The origin's URL, empty when no remote is set. */
  remoteUrl: string
  /** Commits on HEAD; zero on a repository with none. */
  totalCommits: number
  /** The tip of HEAD, null while there are no commits. */
  latestCommit: WorkspaceCommit | null
  /** "public" or "private" when it is known, empty when it is not. */
  visibility: "public" | "private" | ""
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

/** The mode a sub-agent runs in, matching the composer's switch. */
export type AgentMode = "build" | "plan"

/**
 * A sub-agent: a named instruction the user can hand work to, with the mode,
 * MCP connector and models it runs with.
 *
 * The model fields name a provider as well as a model, because a model id alone
 * is ambiguous across providers. The alternative pair is a fallback for when
 * the default model is not available.
 *
 * When used as a primary chat agent, the prompt/systemPrompt REPLACES the global
 * system prompt entirely, and the agent's connector/model/mode are used.
 */
export interface AgentRow {
  id: string
  name: string
  prompt: string
  /** Explicit system prompt replacement (optional, falls back to prompt) */
  systemPrompt: string
  mode: AgentMode
  /** The MCP connectors it may use; empty for none. */
  connectorIds: string[]
  provider: string
  model: string
  altProvider: string
  altModel: string
  createdAt: number
  updatedAt: number
}

/** The fields the Agents page can write. */
export interface AgentInput {
  name: string
  prompt: string
  /** Explicit system prompt replacement (optional, falls back to prompt) */
  systemPrompt: string
  mode: AgentMode
  connectorIds: string[]
  provider: string
  model: string
  altProvider: string
  altModel: string
}

/**
 * One instruction the scheduler will run, once, at a time of its own.
 *
 * It is a timer rather than a cron: the row remembers its run, and giving it a
 * new time arms it again.
 */
export interface ScheduledTask {
  id: string
  /** What the model is asked to do when the time comes. */
  prompt: string
  /** The conversation it speaks in; empty until it has run once. */
  chatId: string
  /** When it is due, in Unix milliseconds. */
  runAt: number
  enabled: boolean
  /** When it last ran, zero while it has not. */
  lastRunAt: number
  /** How the last run went: "" while it has not run, then "ok", "error" or "missed". */
  lastStatus: string
  /** What went wrong, when something did. */
  lastError: string
  createdAt: number
  updatedAt: number
}

/** The fields a new task is booked with. */
export interface ScheduledTaskInput {
  prompt: string
  runAt: number
}

/** What an edit can change about a task. */
export interface ScheduledTaskPatch {
  prompt?: string
  runAt?: number
  enabled?: boolean
}

export interface Usage {
  inputTokens: number
  outputTokens: number
  model: string
}

/** How much was used in one window: request count and the tokens reported. */
export interface UsageTotals {
  requests: number
  inputTokens: number
  outputTokens: number
  totalTokens: number
}

/** One day of the weekly chart. */
export interface UsageDay extends UsageTotals {
  /** The local calendar day, as YYYY-MM-DD. */
  date: string
}

/** One model's free-tier allowance, as listed in model_free_tier_quotas.md. */
export interface UsageModel {
  id: string
  name: string
  rpmLimit: number
  rpdLimit: number
  tpmLimit: number
  status: string
}

/** A provider's usage today, its allowance, and its last seven days. */
export interface UsageProvider extends UsageTotals {
  id: string
  name: string
  enabled: boolean
  /** The daily request allowance summed across the provider's models. */
  rpdLimit: number
  /** The per-minute token allowance summed across its models. */
  tpmLimit: number
  /** Tokens used in the last minute. */
  minute: UsageTotals
  models: UsageModel[]
  daily: UsageDay[]
}

/** Everything the Usage page needs for one refresh. */
export interface UsageSnapshot {
  today: UsageTotals
  minute: UsageTotals
  limits: { rpd: number; tpm: number }
  providers: UsageProvider[]
  daily: UsageDay[]
  /** When the backend assembled the snapshot, in Unix milliseconds. */
  generatedAt: number
}
