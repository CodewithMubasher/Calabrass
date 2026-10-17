import { useState, useEffect, useRef, useMemo, memo } from "react"
import { parseMarkdown } from "@/lib/markdown/parse"
import { renderMdast } from "@/lib/markdown/nodes"

interface MarkdownRendererProps {
  content: string
  isStreaming?: boolean
}

function useThrottledValue<T>(value: T, delay: number): T {
  const [throttled, setThrottled] = useState(value)
  const lastUpdate = useRef(0)
  const rafId = useRef(0)

  useEffect(() => {
    const now = Date.now()
    const elapsed = now - lastUpdate.current

    if (elapsed >= delay) {
      lastUpdate.current = now
      setThrottled(value)
    } else {
      cancelAnimationFrame(rafId.current)
      rafId.current = requestAnimationFrame(() => {
        lastUpdate.current = Date.now()
        setThrottled(value)
      })
    }

    return () => cancelAnimationFrame(rafId.current)
  }, [value, delay])

  return throttled
}

function MarkdownRendererInner({ content, isStreaming = false }: MarkdownRendererProps) {
  const throttledContent = useThrottledValue(content, isStreaming ? 60 : 0)

  const ast = useMemo(() => {
    try {
      return parseMarkdown(throttledContent)
    } catch (e) {
      console.warn("Markdown parse failed, rendering empty:", e)
      return parseMarkdown("")
    }
  }, [throttledContent])

  return <>{renderMdast(ast.children)}</>
}

export const MarkdownRenderer = memo(MarkdownRendererInner)
