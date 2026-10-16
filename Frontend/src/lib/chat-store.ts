import { useSyncExternalStore } from "react"

import type { Chat, ChatMessage } from "@/types/chat"

const STORAGE_KEY = "calabrass-chats"
const TITLE_MAX_LENGTH = 42

/**
 * Canned assistant reply used while there is no backend connected.
 */
export const DEMO_REPLY =
  "This is a placeholder reply from Calabrass. Once the model is connected, real responses will stream in here."

let chats: Chat[] = load()
const listeners = new Set<() => void>()

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

function persist() {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(chats))
  } catch {
    // Ignore quota / private-mode write errors.
  }
}

function emit() {
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return chats
}

/** Subscribe to the chat list. Returns a stable array reference per change. */
export function useChats() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
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
  chats = [chat, ...chats]
  persist()
  emit()
  return chat
}

export function getChat(chatId: string) {
  return chats.find((chat) => chat.id === chatId)
}

/**
 * Append the user message plus a placeholder assistant reply.
 * The chat title is derived from the very first user message.
 */
export function sendMessage(chatId: string, text: string) {
  const clean = text.trim()
  if (!clean) return

  const now = Date.now()
  chats = chats.map((chat) => {
    if (chat.id !== chatId) return chat
    const messages: ChatMessage[] = [
      ...chat.messages,
      { id: newMessageId(), role: "user", content: clean, createdAt: now },
      {
        id: newMessageId(),
        role: "assistant",
        content: DEMO_REPLY,
        createdAt: now + 1,
      },
    ]
    return {
      ...chat,
      messages,
      title: chat.messages.length === 0 ? titleFrom(clean) : chat.title,
      updatedAt: now,
    }
  })
  persist()
  emit()
}

export function deleteChat(chatId: string) {
  chats = chats.filter((chat) => chat.id !== chatId)
  persist()
  emit()
}
