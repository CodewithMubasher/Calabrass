import { useCallback, useEffect, useRef, useState } from "react"

import { rememberChat } from "@/lib/chat-store"
import {
  canRecord,
  openMicrophone,
  VoiceActivityDetector,
  type Microphone,
} from "@/lib/voice/capture"
import { VoicePlayback } from "@/lib/voice/playback"
import { connectVoice, type VoiceEvent, type VoiceSocket } from "@/lib/voice/socket"

/** What the stage is doing. The orb's only control pauses and resumes it. */
export type VoicePhase = "idle" | "listening" | "thinking" | "speaking" | "paused"

/** One thing said in the conversation, shown beside the orb. */
export interface VoiceMessage {
  role: "user" | "assistant"
  text: string
}

export interface VoiceState {
  phase: VoicePhase
  /** The line under the heading: live captions, then the answer. */
  caption: string
  /** Everything said so far, newest last. */
  messages: VoiceMessage[]
  error: string | null
  /** Whether the socket to the backend is open. */
  live: boolean
  supported: boolean
  /** The orb's single control: pause, then resume. */
  toggle: () => void
}

/**
 * Cal Live: a conversation that listens on its own. The microphone opens as soon
 * as the stage does, the detector decides when a turn starts and ends, and every
 * answer is heard while it is still being made. Cal says hello once the
 * microphone is really running, so the stage is never silent. Clicking the orb
 * pauses the whole session; clicking again carries on.
 */
export function useVoice(greeting: string): VoiceState {
  const supported = canRecord()
  const [phase, setPhaseValue] = useState<VoicePhase>("idle")
  const [caption, setCaption] = useState(
    supported
      ? "Tap the orb once — a browser will not let Cal listen until the page has been touched."
      : "Voice needs a browser with microphone access."
  )
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)
  const [messages, setMessages] = useState<VoiceMessage[]>([])

  const phaseRef = useRef<VoicePhase>("idle")
  const socketRef = useRef<VoiceSocket | null>(null)
  // Which connection is current, so a replaced socket cannot report for it.
  const socketGenRef = useRef(0)
  // Whether the socket is open, so a page that hears nothing can say why.
  const liveSocketRef = useRef(false)
  const micRef = useRef<Microphone | null>(null)
  const vadRef = useRef(new VoiceActivityDetector())
  const playbackRef = useRef(new VoicePlayback())
  const replyRef = useRef("")
  const closedRef = useRef(false)

  // Audio is only "running" once a frame has actually arrived. A browser that has
  // not been touched keeps the context suspended and produces nothing, so the
  // microphone is retried until it is heard — that is what makes the first click
  // open it, and what lets pause and resume behave.
  const liveRef = useRef(false)
  // A microphone that is thrown away while it is still opening must not be
  // handed over afterwards, or the device stays open behind the next one.
  const micGenRef = useRef(0)
  // While Cal is saying hello the detector is deaf, so her own voice cannot be
  // mistaken for the visitor's. Muted until she finishes — or, in case the
  // socket never reports it, for a few seconds.
  const mutedUntilRef = useRef(0)
  // The greeting belongs to the visit, not to each time the microphone is
  // started: pausing and carrying on must not say hello again.
  const greetedRef = useRef(false)
  // When the microphone last heard anything at all, so a dead input can be
  // reported instead of looking like a detector that never fires.
  const quietSinceRef = useRef(0)
  // When the listener last put words on the page, so the status line does not
  // wipe out the words still arriving.
  const heardAtRef = useRef(0)
  // The greeting is part of the conversation, but only once.
  const greetingRef = useRef(false)

  // One setter keeps the ref and the state together, so the callbacks that read
  // the phase never go stale.
  const setPhase = useCallback((next: VoicePhase) => {
    phaseRef.current = next
    setPhaseValue(next)
  }, [])

  // --- events from the backend ---------------------------------------------

  const handleEvent = useCallback(
    (event: VoiceEvent) => {
      switch (event.type) {
        case "transcript":
          // Live captions: what has been heard so far. The words themselves are
          // the line while they arrive, so the status underneath stays out of
          // the way until the room goes quiet again.
          heardAtRef.current = Date.now()
          setCaption(event.text)
          break
        case "user":
          // A new turn has been heard, so the previous answer is over: whatever
          // arrives next belongs to this reply and may play. The question is part
          // of the conversation now, so it is not repeated as the live line.
          playbackRef.current.reset()
          setCaption("")
          setMessages((current) =>
            [...current, { role: "user" as const, text: event.user }].slice(-24)
          )
          break
        case "token":
          replyRef.current += event.text
          setCaption(replyRef.current)
          break
        case "audio":
          // Audio only arrives while Cal is talking, greeting or answer alike.
          if (event.audio) playbackRef.current.push(event.audio)
          setPhase("speaking")
          break
        case "spoken":
          // The greeting is done: the visitor can be listened to from here, and
          // what Cal just said is part of the conversation like anything else.
          mutedUntilRef.current = 0
          if (!greetingRef.current) {
            greetingRef.current = true
            setMessages((current) =>
              [...current, { role: "assistant" as const, text: greeting }].slice(-24)
            )
          }
          setPhase("listening")
          break
        case "chat":
          // The conversation is a chat, so the sidebar can show it.
          if (event.chat) rememberChat(event.chat)
          break
        case "done":
          replyRef.current = event.text || replyRef.current
          // The answer belongs to the history now, not to the line still being
          // written.
          setCaption("")
          setMessages((current) =>
            [...current, { role: "assistant" as const, text: replyRef.current }].slice(-24)
          )
          setPhase("listening")
          break
        case "error":
          console.warn("[voice] " + event.message)
          setError(event.message)
          setPhase("listening")
          break
      }
    },
    [greeting, setPhase]
  )

  // --- the socket -----------------------------------------------------------

  // Held in a ref so a dropped socket can call back into the connector without
  // the callback having to know it is recursive.
  const connectRef = useRef<() => void>(() => {})

  const connectSocket = useCallback(() => {
    if (closedRef.current) return
    // Each connection is numbered so a socket that has been replaced cannot
    // report state, or ask for a retry, on behalf of the one that is current.
    const generation = ++socketGenRef.current
    const socket = connectVoice(handleEvent, (open) => {
      if (generation !== socketGenRef.current) return
      liveSocketRef.current = open
      setLive(open)
      console.info(open ? "[voice] socket connected" : "[voice] socket closed")
      if (open) {
        if (liveRef.current && phaseRef.current === "listening") setCaption("Listening…")
        return
      }
      if (!closedRef.current && phaseRef.current !== "paused") {
        window.setTimeout(() => {
          if (!closedRef.current && generation === socketGenRef.current) connectRef.current()
        }, 1500)
      }
    })
    socketRef.current = socket
  }, [handleEvent])

  useEffect(() => {
    connectRef.current = connectSocket
  }, [connectSocket])

  // --- the microphone -------------------------------------------------------

  const stopMic = useCallback(() => {
    micGenRef.current += 1
    micRef.current?.stop()
    micRef.current = null
    liveRef.current = false
  }, [])

  const startMic = useCallback(() => {
    if (!supported || liveRef.current) return

    stopMic()
    vadRef.current = new VoiceActivityDetector()
    setError(null)

    // Anything audible is sent, not only what the detector calls speech: the
    // detector decides where a turn ends, and waiting for it before sending
    // costs the first words of every question. Quiet frames are held back — a
    // listener asked to transcribe nothing but silence eventually stops
    // answering at all.
    const preRoll: ArrayBuffer[] = []
    let preRollSamples = 0
    const preRollMax = 16000 * 0.3
    const generation = micGenRef.current
    let lastLog = 0

    void openMicrophone((pcm) => {
      if (generation !== micGenRef.current) return

      // The first frame is the moment audio is really running.
      if (!liveRef.current) {
        liveRef.current = true
        setPhase("listening")
        setCaption("Listening…")
        quietSinceRef.current = 0
        console.info("[voice] microphone is running")
        // Cal says hello once the microphone is live, and the detector stays
        // deaf until she has finished.
        if (!greetedRef.current) {
          greetedRef.current = true
          mutedUntilRef.current = Date.now() + 8000
          socketRef.current?.speak(greeting)
        }
      }

      if (Date.now() < mutedUntilRef.current) return

      const result = vadRef.current.push(pcm)
      const level = vadRef.current.lastLevel

      // When the microphone last heard anything at all. A dead input and a
      // detector that never fires look identical from here; this is what tells
      // them apart.
      if (level > 0.0008) quietSinceRef.current = 0
      else if (quietSinceRef.current === 0) quietSinceRef.current = Date.now()

      // Every couple of seconds the page says what it hears and where it is
      // sending it: a quiet microphone, a detector that never fires and a
      // connection that never opened are indistinguishable otherwise.
      if (Date.now() - lastLog > 2000) {
        lastLog = Date.now()
        const vad = vadRef.current
        console.info(
          "[voice] %s level %s, a turn opens at %s, floor %s, %s",
          liveSocketRef.current ? "connected," : "NOT connected,",
          level.toFixed(4),
          vad.startThreshold.toFixed(4),
          vad.floorLevel.toFixed(4),
          vad.isSpeaking ? "speaking" : "quiet"
        )

        if (phaseRef.current === "listening" && Date.now() - heardAtRef.current > 2500) {
          if (!liveSocketRef.current) {
            setCaption("Not connected to the backend — is the server running?")
          } else if (quietSinceRef.current !== 0 && Date.now() - quietSinceRef.current > 3000) {
            setCaption("Listening, but the microphone is silent — check the input device.")
          } else {
            setCaption("Listening…")
          }
        }
      }

      if (result === "start") {
        // Speaking over Cal cuts the answer short.
        if (phaseRef.current === "speaking") {
          playbackRef.current.stop()
          socketRef.current?.cancel()
        }
        setPhase("listening")
        replyRef.current = ""
        setError(null)
        quietSinceRef.current = 0
        console.info("[voice] speech started", level.toFixed(4))
      } else if (result === "end") {
        socketRef.current?.endUtterance()
        setPhase("thinking")
        console.info("[voice] speech ended")
      }

      const socket = socketRef.current
      if (!socket) return

      // Nothing is sent while Cal is talking: the microphone hears her through
      // the speakers, and that answer must not be taken for the next question.
      // The detector above still runs, so interrupting her keeps working.
      if (phaseRef.current === "speaking") {
        preRoll.length = 0
        preRollSamples = 0
        return
      }

      // A very low bar, not the speech threshold: whatever reaches the
      // microphone goes to the listener, so a quiet one still gets heard.
      if (!vadRef.current.isSpeaking && level <= 0.0006) {
        preRoll.push(pcm)
        preRollSamples += new Int16Array(pcm).length
        while (preRollSamples > preRollMax && preRoll.length > 1) {
          preRollSamples -= new Int16Array(preRoll.shift() as ArrayBuffer).length
        }
        return
      }

      for (const buffered of preRoll) socket.sendPcm(buffered)
      preRoll.length = 0
      preRollSamples = 0
      socket.sendPcm(pcm)
    })
      .then((mic) => {
        if (generation !== micGenRef.current) {
          mic.stop()
          return
        }
        micRef.current = mic
      })
      .catch((err: unknown) => {
        if (generation !== micGenRef.current) return
        liveRef.current = false
        setPhase("idle")
        console.error("[voice] the microphone could not be opened", err)
        setError(
          err instanceof DOMException && err.name === "NotAllowedError"
            ? "Microphone blocked — allow access and reload."
            : "Could not reach the microphone."
        )
      })
  }, [greeting, supported, setPhase, stopMic])

  // --- the orb's control ----------------------------------------------------

  const toggle = useCallback(() => {
    if (phaseRef.current === "paused") {
      playbackRef.current.reset()
      setPhase("listening")
      startMic()
      return
    }

    // Not running yet: this click is the gesture the browser needs before it will
    // let audio flow, so it opens the microphone rather than pausing it.
    if (!liveRef.current) {
      startMic()
      return
    }

    // Running: pause the session until the orb is clicked again.
    stopMic()
    playbackRef.current.stop()
    socketRef.current?.cancel()
    setPhase("paused")
    setCaption("Paused — tap the orb to carry on")
  }, [setPhase, startMic, stopMic])

  // --- lifecycle ------------------------------------------------------------

  useEffect(() => {
    if (!supported) return
    // The effect runs again after its own cleanup in development, and the
    // connection made then has to be allowed to live: leaving this set would
    // refuse every socket for the rest of the visit.
    closedRef.current = false
    connectSocket()
    startMic()

    const playback = playbackRef.current
    return () => {
      closedRef.current = true
      socketGenRef.current += 1
      liveSocketRef.current = false
      stopMic()
      playback.stop()
      socketRef.current?.close()
      socketRef.current = null
    }
  }, [supported, connectSocket, startMic, stopMic])

  return { phase, caption, messages, error, live, supported, toggle }
}
