import type { Chat } from "@/types/chat"

/** The live voice socket: PCM in, transcript, tokens and audio out. */

export type VoiceEvent =
  | { type: "transcript"; text: string; final?: boolean }
  | { type: "user"; user: string }
  | { type: "token"; text: string }
  | { type: "audio"; text: string; audio?: string }
  | { type: "spoken" }
  | { type: "chat"; chat?: Chat }
  | { type: "trace"; trace?: unknown }
  | { type: "done"; text: string; chat?: Chat }
  | { type: "error"; message: string }

export interface VoiceSocket {
  /** One frame of microphone audio. */
  sendPcm: (pcm: ArrayBuffer) => void
  /** The user stopped speaking: answer what was heard. */
  endUtterance: () => void
  /** The user started speaking over Cal. */
  cancel: () => void
  /** Read a line aloud without it being a turn: the greeting. */
  speak: (text: string) => void
  close: () => void
}

export function connectVoice(
  onEvent: (event: VoiceEvent) => void,
  onState?: (open: boolean) => void
): VoiceSocket {
  const socket = new WebSocket(
    `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/voice`
  )

  // Control sent while the socket is still opening is held, not dropped, so the
  // greeting is not lost to a slow connection. Audio is never held back: a
  // stale frame is worse than none.
  const held: string[] = []

  // A socket that is thrown away while it is still opening must stay silent:
  // nothing it has held back may be sent, and it may not report state for the
  // connection that has replaced it.
  let abandoned = false

  socket.onmessage = (event: MessageEvent<string>) => {
    if (abandoned) return
    try {
      onEvent(JSON.parse(event.data) as VoiceEvent)
    } catch {
      // A frame we do not understand is ignored rather than breaking the call.
    }
  }
  socket.onopen = () => {
    if (abandoned) return
    onState?.(true)
    for (const message of held.splice(0)) socket.send(message)
  }
  socket.onclose = () => {
    if (abandoned) return
    onState?.(false)
  }

  const send = (message: object) => {
    const text = JSON.stringify(message)
    if (socket.readyState === WebSocket.OPEN) socket.send(text)
    else if (socket.readyState === WebSocket.CONNECTING) held.push(text)
  }

  return {
    sendPcm: (pcm) => {
      if (!abandoned && socket.readyState === WebSocket.OPEN) socket.send(pcm)
    },
    endUtterance: () => send({ type: "utterance_end" }),
    cancel: () => send({ type: "cancel" }),
    speak: (text) => send({ type: "speak", text }),
    close: () => {
      abandoned = true
      if (socket.readyState === WebSocket.CONNECTING) {
        // Closing a socket that is still opening is the one case the browser
        // complains about, and it is a socket nobody is listening to: close it
        // the moment it opens instead.
        socket.addEventListener("open", () => socket.close())
        return
      }
      socket.close()
    },
  }
}
