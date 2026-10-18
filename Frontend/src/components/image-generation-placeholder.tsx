import { useEffect, useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon } from "@hugeicons/react"
import { Image01Icon } from "@hugeicons/core-free-icons"

/**
 * Image Generation Placeholder Card - Minimal Version
 * 
 * Just a clean card with shimmer effect, no text, no icons, no animations.
 * After 5 seconds → loads random image from picsum.photos/512/512.
 */
export function ImageGenerationPlaceholder({
  prompt = "A beautiful scene",
  onComplete,
}: {
  /** The prompt that triggered the generation (for display) */
  prompt?: string
  /** Called when the image has finished loading */
  onComplete?: (imageUrl: string) => void
}) {
  const [phase, setPhase] = useState<"placeholder" | "loading" | "complete">("placeholder")
  const [imageUrl, setImageUrl] = useState<string>("")

  // Inject shimmer keyframes once
  useEffect(() => {
    const style = document.createElement("style")
    style.textContent = `
      @keyframes shimmer {
        0% { background-position: -200% -200%; }
        100% { background-position: 200% 200%; }
      }
      @keyframes fade-in {
        from { opacity: 0; transform: scale(0.98); }
        to { opacity: 1; transform: scale(1); }
      }
    `
    document.head.appendChild(style)
    return () => {
      document.head.removeChild(style)
    }
  }, [])

  useEffect(() => {
    // After 5 seconds, load the image
    const timeout = setTimeout(() => {
      setPhase("loading")
      
      // Generate a random image URL from picsum.photos (square 512x512)
      const randomId = Math.floor(Math.random() * 1000)
      const url = `https://picsum.photos/512/512?random=${randomId}&t=${Date.now()}`
      
      const img = new Image()
      img.onload = () => {
        setImageUrl(url)
        setPhase("complete")
        onComplete?.(url)
      }
      img.onerror = () => {
        // Fallback if picsum fails
        const fallbackUrl = `https://picsum.photos/512/512?random=${randomId + 1}&t=${Date.now()}`
        setImageUrl(fallbackUrl)
        setPhase("complete")
        onComplete?.(fallbackUrl)
      }
      img.src = url
    }, 5000)

    return () => clearTimeout(timeout)
  }, [onComplete])

  // Shimmer animation style - diagonal from top-left to bottom-right, subtle
  const shimmerStyle = {
    background: "linear-gradient(135deg, transparent 40%, rgba(255,255,255,0.04) 50%, transparent 60%)",
    backgroundSize: "200% 200%",
    animation: "shimmer 2s infinite",
  }

  if (phase === "complete" && imageUrl) {
    return (
      <div className="relative group">
        <img
          src={imageUrl}
          alt={`Generated: ${prompt}`}
          className="w-64 h-64 rounded-xl shadow-xl border border-white/10 animate-fade-in object-cover"
          loading="lazy"
        />
        <div className="absolute bottom-2 right-2 flex items-center gap-1 px-2 py-1 rounded-full bg-black/60 backdrop-blur-sm text-xs text-white/80">
          <HugeiconsIcon icon={Image01Icon} strokeWidth={2} className="size-3.5" />
          <span>Generated</span>
        </div>
      </div>
    )
  }

  return (
    <div className="relative w-64 h-64 flex-shrink-0">
      {/* Clean card with just shimmer effect */}
      <div
        className={cn(
          "relative w-full h-full rounded-xl border border-white/10 bg-white/5",
          "overflow-hidden transition-all duration-500",
          phase === "loading" && "opacity-50 scale-[0.98]"
        )}
      >
        {/* Shimmer sweep effect - the only animation */}
        <div
          className="absolute inset-0 pointer-events-none"
          style={shimmerStyle}
        />
      </div>
    </div>
  )
}

export default ImageGenerationPlaceholder