import type { CSSProperties } from "react"
import "./cal-live-orb.css"

// The logo palette, left → right, shared with the page background.
const PALETTE = ["#6BFFE0", "#5B7FFF", "#A07CFF", "#FF8FB8"]

// The smoke runs a touch cooler and brighter than the logo blue, so the ball
// reads as sky blue and pink instead of indigo.
const SMOKE = ["#6BFFE0", "#57BDFF", "#A07CFF", "#FF8FB8"]

// Bright wisps that ride on top of the colour: pale blue, sky blue, pink.
const WISPS = ["#EAF6FF", "#7FD0FF", "#FF9ECB"]

/**
 * The ring is two arcs on a 100×100 viewBox at r = 48, split at 12 o'clock so
 * the logo can fall through the opening. The seal is the missing 70°, drawn
 * once the logo has landed.
 */
const RING_ARC = "M 77.53 10.68 A 48 48 0 1 1 22.47 10.68"
const RING_SEAL = "M 22.47 10.68 A 48 48 0 0 1 77.53 10.68"

const prefersReducedMotion = () =>
  typeof window !== "undefined" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches

/**
 * The Cal Live orb: a luminous ball of churning smoke, a thin ring hugging
 * its edge, and the logo dropping in. `liveNoise` lets the smoke itself morph
 * (best look); turn it off on weak devices to keep only the drifting motion.
 */
export function CalLiveOrb({ liveNoise = true }: { liveNoise?: boolean }) {
  const morph = liveNoise && !prefersReducedMotion()

  return (
    <div className="cal-live-orb" aria-hidden="true">
      <div className="cal-live-orb__glow" />

      <div className="cal-live-orb__waves">
        {/* Everything inside is pushed through a noise map, which is what
            tears soft blobs into wisps of smoke. */}
        <div className="cal-live-orb__smoke">
          <div className="cal-live-orb__base" />

          <div className="cal-live-orb__layer cal-live-orb__layer--a">
            {SMOKE.map((color, index) => (
              <span
                key={color}
                className={`cal-live-orb__wave cal-live-orb__wave--${index + 1}`}
                style={{ "--c": color } as CSSProperties}
              />
            ))}
          </div>

          <div className="cal-live-orb__layer cal-live-orb__layer--b">
            {WISPS.map((color, index) => (
              <span
                key={color}
                className={`cal-live-orb__wisp cal-live-orb__wisp--${index + 1}`}
                style={{ "--c": color } as CSSProperties}
              />
            ))}
          </div>
        </div>
      </div>

      <svg className="cal-live-orb__ring" viewBox="0 0 100 100">
        <defs>
          <linearGradient id="cal-live-ring" x1="0" y1="1" x2="1" y2="0">
            <stop offset="0%" stopColor={PALETTE[0]} />
            <stop offset="38%" stopColor={PALETTE[1]} />
            <stop offset="70%" stopColor={PALETTE[2]} />
            <stop offset="100%" stopColor={PALETTE[3]} />
          </linearGradient>

          <filter
            id="cal-live-smoke"
            x="-20%"
            y="-20%"
            width="140%"
            height="140%"
            colorInterpolationFilters="sRGB"
          >
            <feTurbulence
              type="fractalNoise"
              baseFrequency="0.011 0.017"
              numOctaves="3"
              seed="7"
              result="noise"
            >
              {morph && (
                <animate
                  attributeName="baseFrequency"
                  dur="7s"
                  repeatCount="indefinite"
                  values="0.011 0.017; 0.017 0.011; 0.010 0.020; 0.011 0.017"
                />
              )}
            </feTurbulence>
            <feDisplacementMap
              in="SourceGraphic"
              in2="noise"
              scale="110"
              xChannelSelector="R"
              yChannelSelector="G"
            />
          </filter>
        </defs>
        <path className="cal-live-orb__ring-arc" d={RING_ARC} pathLength={290} />
        <path
          className="cal-live-orb__ring-seal"
          d={RING_SEAL}
          pathLength={70}
        />
        <circle
          className="cal-live-orb__ring-flash"
          cx="50"
          cy="50"
          r="48"
        />
      </svg>

      <div className="cal-live-orb__logo-wrap">
        <div className="cal-live-orb__logo-drop">
          <img src="/vite.svg" alt="" className="cal-live-orb__logo" />
        </div>
      </div>
    </div>
  )
}

export default CalLiveOrb
