/**
 * Plays Cal's answer as it arrives, chunk by chunk, instead of waiting for the
 * whole reply. A MediaSource is what lets an mp3 be played while it is still
 * being made.
 */
export class VoicePlayback {
  private source: MediaSource | null = null
  private buffer: SourceBuffer | null = null
  private audio = new Audio()
  private pending: string[] = []
  private appending = false
  private started = false
  private stopped = false

  /** True once the first audio has arrived. */
  get playing(): boolean {
    return this.started
  }

  /** Hands over one mp3 chunk; playback follows as soon as the buffer allows. */
  push(base64: string) {
    if (this.stopped) return
    if (!this.source) this.open()
    this.pending.push(base64)
    void this.drain()
  }

  private open() {
    this.source = new MediaSource()
    this.audio.src = URL.createObjectURL(this.source)
    this.audio.autoplay = true
    this.source.addEventListener("sourceopen", () => {
      if (!this.source) return
      this.buffer = this.source.addSourceBuffer("audio/mpeg")
      this.buffer.addEventListener("updateend", () => {
        this.appending = false
        void this.drain()
      })
      void this.drain()
    })
  }

  private async drain() {
    if (this.appending || !this.buffer || this.pending.length === 0) return

    const chunk = this.pending.shift()
    if (!chunk) return

    this.appending = true
    try {
      this.buffer.appendBuffer(decode(chunk))
      if (!this.started) {
        this.started = true
        // A click has already happened on this page, so this may start at once.
        void this.audio.play().catch(() => {})
      }
    } catch {
      this.appending = false
    }
  }

  /** Stops the answer: the barge-in path. */
  stop() {
    this.stopped = true
    this.started = false
    this.audio.pause()
    this.audio.removeAttribute("src")
    this.audio.load()
    this.pending = []
    if (this.source) {
      try {
        this.source.endOfStream()
      } catch {
        // Already ended.
      }
      this.source = null
    }
    this.buffer = null
  }

  /** Lets the next answer play. */
  reset() {
    this.stopped = false
  }
}

function decode(base64: string): Uint8Array<ArrayBuffer> {
  const binary = atob(base64)
  const bytes = new Uint8Array(new ArrayBuffer(binary.length))
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}
