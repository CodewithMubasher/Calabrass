import { useCallback, useEffect, useRef, useState } from "react"

/** Voices that sound closest to a friendly assistant, in order of preference. */
const PREFERRED_VOICES = [
  "Google US English",
  "Google UK English Female",
  "Samantha",
  "Microsoft Aria",
  "Microsoft Jenny",
  "Microsoft Zira",
]

interface VoiceState {
  /** Smoothed 0–1 loudness, handed to the orb as `--level`. */
  level: number
  /** Where the level is heading; decays in the gaps between words. */
  target: number
  raf: number
  /** Fallback word rhythm, used when the browser fires no boundary events. */
  pulse: number | null
  sawBoundary: boolean
}

function pickVoice(): SpeechSynthesisVoice | null {
  const voices = window.speechSynthesis.getVoices()
  if (voices.length === 0) return null

  const english = voices.filter((voice) =>
    voice.lang.toLowerCase().startsWith("en"),
  )
  const pool = english.length > 0 ? english : voices

  for (const name of PREFERRED_VOICES) {
    const match = pool.find((voice) => voice.name.includes(name))
    if (match) return match
  }

  return pool.find((voice) => voice.default) ?? pool[0]
}

/**
 * Speaks `text` out loud and reports how loud it is word by word as a `--level`
 * custom property (0–1) on `hostRef`.
 *
 * Speech synthesis cannot be routed through the Web Audio API, so there is no
 * waveform to measure. Instead every word boundary bumps the level, which then
 * decays through the gap before the next word — close enough to read as volume.
 * Browsers that never report boundaries fall back to a timed rhythm.
 *
 * Returns `speak` so a click can replay it, and `speaking` for the caption.
 */
export function useSpeech(text: string, startDelayMs = 1700) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const [speaking, setSpeaking] = useState(false)
  // Held so the utterance is not collected mid-speech (Chrome drops the tail).
  const utteranceRef = useRef<SpeechSynthesisUtterance | null>(null)
  const pendingRef = useRef(0)
  const stateRef = useRef<VoiceState>({
    level: 0,
    target: 0,
    raf: 0,
    pulse: null,
    sawBoundary: false,
  })

  const supported =
    typeof window !== "undefined" &&
    "speechSynthesis" in window &&
    "SpeechSynthesisUtterance" in window

  const paint = useCallback(() => {
    // Declared as a function so the loop can re-arm itself; hoisting keeps the
    // recursion legal for the hooks compiler.
    function step() {
      const state = stateRef.current
      // Bleed off between words, then ease the level toward the target, so each
      // word lands as an attack and relaxes into the gap behind it.
      state.target *= 0.98
      state.level += (state.target - state.level) * 0.3

      if (state.target < 0.01 && state.level < 0.01) {
        state.raf = 0
        state.level = 0
        hostRef.current?.style.removeProperty("--level")
        return
      }

      hostRef.current?.style.setProperty("--level", state.level.toFixed(3))
      state.raf = window.requestAnimationFrame(step)
    }

    step()
  }, [])

  const kick = useCallback(() => {
    if (!stateRef.current.raf) {
      stateRef.current.raf = window.requestAnimationFrame(paint)
    }
  }, [paint])

  const settle = useCallback(() => {
    const state = stateRef.current
    state.target = 0
    if (state.pulse !== null) {
      window.clearInterval(state.pulse)
      state.pulse = null
    }
    kick()
  }, [kick])

  const speak = useCallback(() => {
    if (!supported) return

    const synth = window.speechSynthesis
    const state = stateRef.current
    synth.cancel()

    state.sawBoundary = false
    if (state.pulse !== null) {
      window.clearInterval(state.pulse)
      state.pulse = null
    }

    const utterance = new SpeechSynthesisUtterance(text)
    const voice = pickVoice()
    if (voice) utterance.voice = voice
    utterance.rate = 0.97
    utterance.pitch = 1.02

    utterance.onstart = () => {
      setSpeaking(true)
      state.target = 0.8
      kick()
      // Keep the mouth moving if this browser stays quiet about boundaries.
      state.pulse = window.setInterval(() => {
        if (state.sawBoundary) {
          if (state.pulse !== null) window.clearInterval(state.pulse)
          state.pulse = null
          return
        }
        state.target = 0.45 + Math.random() * 0.55
        kick()
      }, 170)
    }
    utterance.onboundary = () => {
      state.sawBoundary = true
      state.target = 0.5 + Math.random() * 0.5
      kick()
    }
    utterance.onend = () => {
      setSpeaking(false)
      settle()
    }
    utterance.onerror = () => {
      setSpeaking(false)
      settle()
    }

    utteranceRef.current = utterance
    synth.resume()
    // Chrome sometimes swallows an utterance queued in the same tick as a
    // cancel, so give it a beat.
    window.clearTimeout(pendingRef.current)
    pendingRef.current = window.setTimeout(() => synth.speak(utterance), 60)
  }, [supported, text, kick, settle])

  // Greet once the intro has settled, and clean the voice up on the way out.
  useEffect(() => {
    if (!supported) return
    const state = stateRef.current
    const timer = window.setTimeout(speak, startDelayMs)

    return () => {
      window.clearTimeout(timer)
      window.clearTimeout(pendingRef.current)
      window.speechSynthesis.cancel()
      if (state.pulse !== null) window.clearInterval(state.pulse)
      if (state.raf) window.cancelAnimationFrame(state.raf)
      state.pulse = null
      state.raf = 0
    }
  }, [supported, speak, startDelayMs])

  return { supported, speaking, speak, hostRef }
}
