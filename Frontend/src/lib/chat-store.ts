import { useSyncExternalStore } from "react"

import { api, streamMessage } from "@/lib/api"
import { getSelectedModel } from "@/lib/model-store"
import type { Chat, ChatMessage } from "@/types/chat"

interface ChatState {
  chats: Chat[]
  messages: Record<string, ChatMessage[]>
  streaming: Record<string, string>
  errors: Record<string, string>
  backendError: string | null
  hydrated: boolean
}

let state: ChatState = {
  chats: [],
  messages: {},
  streaming: {},
  errors: {},
  backendError: null,
  hydrated: false,
}

const listeners = new Set<() => void>()

function setState(patch: Partial<ChatState>) {
  state = { ...state, ...patch }
  for (const listener of listeners) listener()
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

// --- selectors --------------------------------------------------------------

export function useChats() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).chats
}

export function useChat(chatId: string) {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).chats.find(
    (chat) => chat.id === chatId
  )
}

export function useChatMessages(chatId: string) {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).messages[chatId]
}

export function useStreamingText(chatId: string) {
  return (
    useSyncExternalStore(subscribe, getSnapshot, getSnapshot).streaming[chatId] ?? ""
  )
}

export function useIsStreaming(chatId: string) {
  return (
    useSyncExternalStore(subscribe, getSnapshot, getSnapshot).streaming[chatId] !==
    undefined
  )
}

export function useChatError(chatId: string) {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).errors[chatId]
}

export function useHydrated() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).hydrated
}

export function useBackendError() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).backendError
}

// --- helpers ----------------------------------------------------------------

function errorMessage(err: unknown) {
  return err instanceof Error ? err.message : "Something went wrong."
}

function without<T>(record: Record<string, T>, key: string) {
  const next: Record<string, T> = {}
  for (const k of Object.keys(record)) {
    if (k !== key) next[k] = record[k]
  }
  return next
}

function upsertChat(chats: Chat[], next: Chat): Chat[] {
  return [next, ...chats.filter((chat) => chat.id !== next.id)].sort(
    (a, b) => b.updatedAt - a.updatedAt
  )
}

function replacePending(list: ChatMessage[], message: ChatMessage): ChatMessage[] {
  const index = list.findIndex((item) => item.pending)
  if (index === -1) return [...list, message]
  const next = [...list]
  next[index] = message
  return next
}

// --- actions ----------------------------------------------------------------

/** Load the sidebar list. Safe to call more than once. */
export async function hydrate() {
  try {
    const chats = await api.listChats()
    setState({ chats, hydrated: true, backendError: null })
  } catch (err) {
    setState({ hydrated: true, backendError: errorMessage(err) })
  }
}

/** Load one conversation, unless we already hold its messages locally. */
export async function ensureChatLoaded(chatId: string) {
  if (state.messages[chatId]) return

  try {
    const detail = await api.getChat(chatId)
    setState({
      chats: upsertChat(state.chats, detail),
      messages: { ...state.messages, [chatId]: detail.messages ?? [] },
      backendError: null,
    })
  } catch (err) {
    setState({
      messages: { ...state.messages, [chatId]: [] },
      errors: { ...state.errors, [chatId]: errorMessage(err) },
    })
  }
}

export async function createChat(): Promise<Chat> {
  const chat = await api.createChat(getSelectedModel())
  setState({
    chats: upsertChat(state.chats, chat),
    messages: { ...state.messages, [chat.id]: [] },
  })
  return chat
}

export async function sendMessage(chatId: string, content: string) {
  const text = content.trim()
  if (!text) return

  const optimistic: ChatMessage = {
    id: `local-${Date.now()}`,
    chatId,
    role: "user",
    content: text,
    createdAt: Date.now(),
    pending: true,
  }

  setState({
    messages: {
      ...state.messages,
      [chatId]: [...(state.messages[chatId] ?? []), optimistic],
    },
    streaming: { ...state.streaming, [chatId]: "" },
    errors: without(state.errors, chatId),
  })

  const selection = getSelectedModel()

  try {
    await streamMessage(
      chatId,
      { content: text, ...(selection ?? {}) },
      {
        onChat: (chat) => setState({ chats: upsertChat(state.chats, chat) }),

        onMessage: (message) =>
          setState({
            messages: {
              ...state.messages,
              [chatId]: replacePending(state.messages[chatId] ?? [], message),
            },
          }),

        onToken: (delta) =>
          setState({
            streaming: {
              ...state.streaming,
              [chatId]: (state.streaming[chatId] ?? "") + delta,
            },
          }),

        onDone: ({ message, interrupted }) =>
          setState({
            messages: {
              ...state.messages,
              [chatId]: [...(state.messages[chatId] ?? []), message],
            },
            streaming: without(state.streaming, chatId),
            errors: interrupted
              ? { ...state.errors, [chatId]: "The reply was interrupted." }
              : without(state.errors, chatId),
          }),

        onError: (message) =>
          setState({
            streaming: without(state.streaming, chatId),
            errors: { ...state.errors, [chatId]: message },
          }),
      }
    )
  } catch (err) {
    setState({
      streaming: without(state.streaming, chatId),
      errors: { ...state.errors, [chatId]: errorMessage(err) },
    })
  }
}

export async function deleteChat(chatId: string) {
  const previous = state.chats
  setState({
    chats: state.chats.filter((chat) => chat.id !== chatId),
    messages: without(state.messages, chatId),
    streaming: without(state.streaming, chatId),
  })

  try {
    await api.deleteChat(chatId)
  } catch (err) {
    setState({
      chats: previous,
      backendError: errorMessage(err),
    })
  }
}
