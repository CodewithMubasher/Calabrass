import { unified } from "unified"
import remarkParse from "remark-parse"
import remarkGfm from "remark-gfm"
import type { Root } from "mdast"

const processor = unified().use(remarkParse).use(remarkGfm)

let cached: { input: string; ast: Root } | null = null

export function parseMarkdown(input: string): Root {
  if (cached && cached.input === input) return cached.ast
  const file = processor.runSync(processor.parse(input))
  const ast = file as Root
  cached = { input, ast }
  return ast
}
