/**
 * The microphone, as 16 kHz mono PCM.
 *
 * A worklet mixes the device's channels down, averages its way to the rate the
 * transcription service wants, and hands the page a frame about every 100 ms.
 * The worklet is loaded from a blob so it needs no extra file in the bundle.
 */

const WORKLET_SOURCE = `
class PcmCapture extends AudioWorkletProcessor {
  constructor() {
    super()
    this.step = sampleRate / 16000
    // About 100 ms. Waking the page for every couple of milliseconds would be
    // wasteful, and a larger frame is what the service expects anyway.
    this.frame = Math.round(16000 * this.step / 10) * 10
    this.buffer = new Float32Array(0)
  }

  process(inputs) {
    const input = inputs[0]
    if (!input || input.length === 0) return true

    // Mix the device's channels down to one.
    const frames = input[0].length
    const mono = new Float32Array(frames)
    for (let i = 0; i < frames; i++) {
      let sum = 0
      for (let channel = 0; channel < input.length; channel++) sum += input[channel][i]
      mono[i] = sum / input.length
    }

    const joined = new Float32Array(this.buffer.length + mono.length)
    joined.set(this.buffer)
    joined.set(mono, this.buffer.length)
    this.buffer = joined

    while (this.buffer.length >= this.frame) {
      this.send(this.buffer.subarray(0, this.frame))
      this.buffer = this.buffer.slice(this.frame)
    }
    return true
  }

  // Averaging each group of samples, rather than picking one out of it, is what
  // stops the high frequencies folding back into the speech that gets heard.
  send(block) {
    const count = Math.floor(block.length / this.step)
    if (count === 0) return

    const pcm = new Int16Array(count)
    for (let i = 0; i < count; i++) {
      const from = Math.floor(i * this.step)
      const to = Math.min(block.length, Math.floor((i + 1) * this.step))
      let sum = 0
      for (let j = from; j < to; j++) sum += block[j]
      const value = (sum / Math.max(1, to - from)) * 32767
      pcm[i] = Math.max(-32768, Math.min(32767, Math.round(value)))
    }
    this.port.postMessage(pcm.buffer, [pcm.buffer])
  }
}

registerProcessor("pcm-capture", PcmCapture)
`

export interface Microphone {
  /** Stops the capture and releases the device. */
  stop: () => void
}

/** Whether this browser can capture the microphone at all. */
export function canRecord(): boolean {
  return (
    typeof navigator !== "undefined" &&
    typeof navigator.mediaDevices?.getUserMedia === "function" &&
    typeof AudioWorkletNode !== "undefined"
  )
}

/**
 * Opens the microphone and hands over 16 kHz mono PCM frames as they are
 * recorded. Nothing is written to disk.
 */
export async function openMicrophone(
  onPcm: (pcm: ArrayBuffer) => void
): Promise<Microphone> {
  const stream = await navigator.mediaDevices.getUserMedia({
    audio: {
      echoCancellation: true,
      noiseSuppression: true,
      // Without this a quiet microphone stays too quiet to be heard at all.
      autoGainControl: true,
    },
  })

  const context = new AudioContext()

  // A browser will not let audio flow until the visitor has touched the page,
  // so a context opened on its own starts suspended and the microphone captures
  // nothing. Resuming on the first gesture is what makes the stage work without
  // asking anyone to click first.
  const resume = () => {
    void context.resume().catch(() => {})
  }
  resume()
  window.addEventListener("pointerdown", resume)
  window.addEventListener("keydown", resume)

  const blob = new Blob([WORKLET_SOURCE], { type: "application/javascript" })
  await context.audioWorklet.addModule(URL.createObjectURL(blob))

  const source = context.createMediaStreamSource(stream)
  const node = new AudioWorkletNode(context, "pcm-capture")
  node.port.onmessage = (event: MessageEvent<ArrayBuffer>) => onPcm(event.data)

  source.connect(node)
  // The worklet pulls its input, so it needs nowhere to send its output; the
  // connection is only what keeps it being asked for audio. It writes silence,
  // so nothing is played back into the room.
  node.connect(context.destination)

  return {
    stop: () => {
      window.removeEventListener("pointerdown", resume)
      window.removeEventListener("keydown", resume)
      node.port.onmessage = null
      node.disconnect()
      source.disconnect()
      stream.getTracks().forEach((track) => track.stop())
      void context.close()
    },
  }
}

/**
 * Decides when the user is speaking. An energy detector with a moving noise
 * floor is enough to open and close a turn, and it costs nothing: no model, no
 * dependency.
 *
 * The thresholds are relative to the quietest level heard and only fall back to
 * fixed ones, so a quiet room, a loud fan or a low-gain microphone all end up
 * with a usable trigger. Every value is exposed, because "it heard nothing" is
 * impossible to tell apart from "it never triggered" without them.
 */
export class VoiceActivityDetector {
  private speaking = false
  private floor = 0.002
  private level = 0
  private loudSince = 0
  private quietSince = 0

  private readonly startAfterMs: number
  private readonly stopAfterMs: number
  private readonly startAt: number
  private readonly stopAt: number

  constructor(startAfterMs = 120, stopAfterMs = 700, startAt = 0.0025, stopAt = 0.0012) {
    this.startAfterMs = startAfterMs
    this.stopAfterMs = stopAfterMs
    this.startAt = startAt
    this.stopAt = stopAt
  }

  /** "start" when speech opens, "end" when it closes, null otherwise. */
  push(pcm: ArrayBuffer, at = Date.now()): "start" | "end" | null {
    const samples = new Int16Array(pcm)
    let sum = 0
    for (let i = 0; i < samples.length; i++) sum += samples[i] * samples[i]
    const rms = Math.sqrt(sum / Math.max(1, samples.length)) / 32768
    this.level = rms

    // The floor follows the quietest level heard, so a loud room does not turn
    // every breath into a turn.
    if (rms < this.floor) this.floor = Math.max(0.0002, rms * 0.6 + this.floor * 0.4)

    if (!this.speaking) {
      if (rms > this.startThreshold) {
        if (this.loudSince === 0) this.loudSince = at
        if (at - this.loudSince >= this.startAfterMs) {
          this.speaking = true
          this.quietSince = 0
          return "start"
        }
      } else {
        this.loudSince = 0
      }
      return null
    }

    if (rms < this.stopThreshold) {
      if (this.quietSince === 0) this.quietSince = at
      if (at - this.quietSince >= this.stopAfterMs) {
        this.speaking = false
        this.loudSince = 0
        return "end"
      }
    } else {
      this.quietSince = 0
    }
    return null
  }

  get isSpeaking(): boolean {
    return this.speaking
  }

  /** The level of the last frame. */
  get lastLevel(): number {
    return this.level
  }

  /** The quietest level heard so far, which the thresholds are built from. */
  get floorLevel(): number {
    return this.floor
  }

  /** The level a frame has to reach to open a turn. */
  get startThreshold(): number {
    return Math.max(this.startAt, this.floor * 3)
  }

  private get stopThreshold(): number {
    return Math.max(this.stopAt, this.floor * 1.5)
  }
}
