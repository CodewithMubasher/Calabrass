import { useSyncExternalStore } from "react"

import type { Chat, ChatMessage } from "@/types/chat"

const STORAGE_KEY = "calabrass-chats"
const TITLE_MAX_LENGTH = 42
const STREAM_TICK_MS = 75

/**
 * Canned assistant reply used while there is no backend connected.
 */
export const DEMO_REPLY = [
  "Here is a placeholder reply from Calabrass.",
  "The model isn't connected yet, so this text is canned and streams in word by word on a timer to mimic real token output.",
  "Once the backend is wired up, actual tokens will arrive over the same channel and this demo loop will be removed.",
  "For now the reply is stored in localStorage, so it survives a refresh and shows up in the Recents list.",
].join(" ")

export interface StreamingReply {
  chatId: string
  messageId: string
  words: string[]
  revealed: number
  rate: number
  startedAt: number
}

interface StoreState {
  chats: Chat[]
  streaming: Record<string, StreamingReply>
}

let state: StoreState = { chats: load(), streaming: {} }
const listeners = new Set<() => void>()
const timers = new Map<string, ReturnType<typeof setInterval>>()

function load(): Chat[] {
  if (typeof window === "undefined") return []
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as Chat[]) : []
  } catch {
    return []
  }
}

function persist(chats: Chat[]) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(chats))
  } catch {
    // Ignore quota / private-mode write errors.
  }
}

function emit() {
  for (const listener of listeners) listener()
}

function setState(next: StoreState) {
  state = next
  emit()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return state
}

function withoutKey<T>(record: Record<string, T>, key: string) {
  const next: Record<string, T> = {}
  for (const k of Object.keys(record)) {
    if (k !== key) next[k] = record[k]
  }
  return next
}

/** Subscribe to the chat list. Returns a stable array reference per change. */
export function useChats() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).chats
}

/** The in-flight reply for a chat, or undefined when nothing is streaming. */
export function useStreamingReply(chatId: string) {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).streaming[chatId]
}

function newMessageId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`
}

/** 14 random digits, e.g. "21831927312371". */
export function createChatId(): string {
  let id = ""
  for (let i = 0; i < 14; i += 1) id += Math.floor(Math.random() * 10)
  return id
}

function titleFrom(text: string) {
  const clean = text.trim().replace(/\s+/g, " ")
  if (!clean) return "New Chat"
  return clean.length > TITLE_MAX_LENGTH
    ? `${clean.slice(0, TITLE_MAX_LENGTH)}...`
    : clean
}

export function createChat(): Chat {
  const now = Date.now()
  const chat: Chat = {
    id: createChatId(),
    title: "New Chat",
    messages: [],
    createdAt: now,
    updatedAt: now,
  }
  setState({ ...state, chats: [chat, ...state.chats] })
  persist(state.chats)
  return chat
}

export function getChat(chatId: string) {
  return state.chats.find((chat) => chat.id === chatId)
}

function cancelStream(chatId: string) {
  const timer = timers.get(chatId)
  if (timer) {
    clearInterval(timer)
    timers.delete(chatId)
  }
}

/** Commit the finished reply as a normal, persisted message. */
function finishStream(chatId: string) {
  cancelStream(chatId)
  const reply = state.streaming[chatId]
  if (!reply) return

  const now = Date.now()
  const chats = state.chats.map((chat) =>
    chat.id === chatId
      ? {
          ...chat,
          messages: [
            ...chat.messages,
            {
              id: reply.messageId,
              role: "assistant" as const,
              content: reply.words.join(" "),
              createdAt: now,
            },
          ],
          updatedAt: now,
        }
      : chat
  )

  persist(chats)
  setState({ chats, streaming: withoutKey(state.streaming, chatId) })
}

/** Simulate a streaming reply with a live tokens/sec readout. */
function startStream(chatId: string) {
  cancelStream(chatId)

  const reply: StreamingReply = {
    chatId,
    messageId: newMessageId(),
    words: DEMO_REPLY.split(" "),
    revealed: 0,
    rate: 0,
    startedAt: Date.now(),
  }
  setState({
    ...state,
    streaming: { ...state.streaming, [chatId]: reply },
  })

  const timer = setInterval(() => {
    const current = state.streaming[chatId]
    if (!current) {
      cancelStream(chatId)
      return
    }

    const step = 1 + Math.floor(Math.random() * 2)
    const revealed = Math.min(current.revealed + step, current.words.length)

    if (revealed >= current.words.length) {
      finishStream(chatId)
      return
    }

    const elapsed = (Date.now() - current.startedAt) / 1000
    setState({
      ...state,
      streaming: {
        ...state.streaming,
        [chatId]: {
          ...current,
          revealed,
          rate: elapsed > 0 ? Math.round(revealed / elapsed) : 0,
        },
      },
    })
  }, STREAM_TICK_MS)

  timers.set(chatId, timer)
}

/**
 * Append the user message, then stream a placeholder assistant reply.
 * The chat title is derived from the very first user message.
 */
export function sendMessage(chatId: string, text: string) {
  const clean = text.trim()
  if (!clean) return

  const now = Date.now()
  const chats = state.chats.map((chat) => {
    if (chat.id !== chatId) return chat
    const messages: ChatMessage[] = [
      ...chat.messages,
      { id: newMessageId(), role: "user", content: clean, createdAt: now },
    ]
    return {
      ...chat,
      messages,
      title: chat.messages.length === 0 ? titleFrom(clean) : chat.title,
      updatedAt: now,
    }
  })

  persist(chats)
  setState({ ...state, chats })
  startStream(chatId)
}

export function deleteChat(chatId: string) {
  cancelStream(chatId)
  const chats = state.chats.filter((chat) => chat.id !== chatId)
  persist(chats)
  setState({ chats, streaming: withoutKey(state.streaming, chatId) })
}
