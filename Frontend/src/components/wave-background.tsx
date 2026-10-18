import { memo, type CSSProperties } from "react"
import "./wave-background.css"

// Logo colors, left → right
const DEFAULT_COLORS = ["#6BFFE0", "#5B7FFF", "#A07CFF", "#FF8FB8"]

interface WaveBackgroundProps {
  /** Array of up to 4 colors (defaults to logo palette) */
  colors?: string[]
  /** Cover the whole viewport instead of the parent */
  fixed?: boolean
  /**
   * A low band along the bottom of the screen rather than a field over the
   * whole of it. What a phone gets: the top of a tall, narrow screen has to
   * stay black for the heading, and the one place with room to glow is the foot
   * of it, where the composer is docked.
   */
  band?: boolean
  /** Animation speed multiplier (1 = default, 2 = twice as fast) */
  speed?: number
  /** Background base color */
  base?: string
  className?: string
}

/**
 * Animated wave glow background. Pure CSS (GPU transforms only), zero JS per frame.
 *
 * Usage:
 *   <div style={{ position: "relative" }}>
 *     <WaveBackground />
 *     <YourContent />            // give content position: relative; z-index: 1
 *   </div>
 */
function WaveBackground({
  colors = DEFAULT_COLORS,
  fixed = false,
  band = false,
  speed = 1,
  base = "#000",
  className = "",
}: WaveBackgroundProps) {
  return (
    <div
      className={`wave-bg ${fixed ? "wave-bg--fixed" : ""} ${band ? "wave-bg--band" : ""} ${className}`}
      style={{ "--wave-base": base, "--wave-speed": speed } as CSSProperties}
      aria-hidden="true"
    >
      {colors.slice(0, 4).map((color, i) => (
        <span
          key={i}
          className={`wave-bg__blob wave-bg__blob--${i + 1}`}
          style={{ "--c": color } as CSSProperties}
        />
      ))}
      <span className="wave-bg__shade" />
    </div>
  )
}

export default memo(WaveBackground)
