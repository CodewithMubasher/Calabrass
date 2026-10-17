import ReactMarkdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"

import { cn } from "cn"

/**
 * Element styling for rendered replies. Kept at module scope so the object
 * identity is stable across renders.
 *
 * react-markdown builds React elements instead of injecting HTML strings, so
 * model output cannot introduce markup, and URLs are sanitised by default.
 * remark-gfm adds tables, task lists and strikethrough on top of CommonMark.
 */
const components: Components = {
  h1: ({ children }) => (
    <h1 className="mt-4 mb-2 text-lg font-semibold text-white first:mt-0">
      {children}
    </h1>
  ),
  h2: ({ children }) => (
    <h2 className="mt-4 mb-2 text-base font-semibold text-white first:mt-0">
      {children}
    </h2>
  ),
  h3: ({ children }) => (
    <h3 className="mt-3 mb-1.5 text-sm font-semibold text-white first:mt-0">
      {children}
    </h3>
  ),
  h4: ({ children }) => (
    <h4 className="mt-3 mb-1.5 text-sm font-medium text-white first:mt-0">
      {children}
    </h4>
  ),
  p: ({ children }) => (
    <p className="my-2 leading-relaxed first:mt-0 last:mb-0">{children}</p>
  ),
  ul: ({ children }) => (
    <ul className="my-2 list-disc space-y-1 pl-5 marker:text-white/40 first:mt-0 last:mb-0">
      {children}
    </ul>
  ),
  ol: ({ children }) => (
    <ol className="my-2 list-decimal space-y-1 pl-5 marker:text-white/40 first:mt-0 last:mb-0">
      {children}
    </ol>
  ),
  li: ({ children }) => <li className="pl-0.5">{children}</li>,
  a: ({ href, children }) => (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="text-sky-400 underline underline-offset-2 hover:text-sky-300"
    >
      {children}
    </a>
  ),
  strong: ({ children }) => (
    <strong className="font-semibold text-white">{children}</strong>
  ),
  em: ({ children }) => <em className="italic">{children}</em>,
  del: ({ children }) => (
    <del className="text-white/50 line-through">{children}</del>
  ),
  blockquote: ({ children }) => (
    <blockquote className="my-2 border-l-2 border-white/20 pl-3 text-white/70 italic">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="my-4 border-white/10" />,
  // Fenced code lands inside <pre>, so the inline chip styling is reset there
  // rather than by trying to detect the difference in the `code` override.
  pre: ({ children }) => (
    <pre className="scrollbar-thin my-2 overflow-x-auto rounded-lg border border-white/10 bg-black/40 p-3 text-xs leading-relaxed [&>code]:rounded-none [&>code]:bg-transparent [&>code]:p-0 [&>code]:text-[1.05em] [&>code]:text-white/90">
      {children}
    </pre>
  ),
  code: ({ children, className }) => (
    <code
      className={cn(
        "rounded bg-white/10 px-1 py-0.5 font-mono text-[0.85em] text-sky-200",
        className
      )}
    >
      {children}
    </code>
  ),
  table: ({ children }) => (
    <div className="scrollbar-thin my-2 w-full overflow-x-auto">
      <table className="w-full border-collapse text-xs">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="bg-white/5">{children}</thead>,
  th: ({ children }) => (
    <th className="border border-white/10 px-3 py-1.5 text-left font-medium whitespace-nowrap text-white">
      {children}
    </th>
  ),
  td: ({ children }) => (
    <td className="border border-white/10 px-3 py-1.5 align-top">{children}</td>
  ),
  // GFM renders task-list items as disabled checkboxes.
  input: ({ type, checked, ...props }) =>
    type === "checkbox" ? (
      <input
        type="checkbox"
        checked={checked}
        readOnly
        disabled
        className="mr-1.5 size-3.5 translate-y-[2px] accent-sky-500"
      />
    ) : (
      <input type={type} {...props} />
    ),
  img: ({ src, alt }) => (
    <img
      src={src}
      alt={alt ?? ""}
      loading="lazy"
      className="my-2 max-w-full rounded-lg border border-white/10"
    />
  ),
}

/**
 * Renders an assistant reply as Markdown.
 *
 * Pass `className="streaming-caret"` while the reply is still arriving to pin a
 * blinking caret to the end of the last rendered block.
 */
export function Markdown({
  children,
  className,
}: {
  children: string
  className?: string
}) {
  return (
    <div
      className={cn(
        "min-w-0 text-base leading-relaxed wrap-break-word",
        className
      )}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  )
}
