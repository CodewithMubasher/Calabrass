import { CodeBlock } from "@/components/code-block"
import type { Root, Content, PhrasingContent } from "mdast"

function renderChildren(node: { children: unknown[] }): React.ReactNode {
  return (node.children as Content[]).map((child, i) => (
    <NodeRenderer key={i} node={child} />
  ))
}

function renderPhrasing(node: { children: unknown[] }): React.ReactNode {
  return (node.children as PhrasingContent[]).map((child, i) => (
    <NodeRenderer key={i} node={child} />
  ))
}

/**
 * Maps the mdast tree to plain elements. It carries no styling of its own:
 * shadcn/typeset (the `typeset typeset-chat` container the reply is wrapped in)
 * styles everything inside — headings, paragraphs, lists, tables, quotes, code —
 * so a class here would only fight it. Tailwind utilities sit in the `utilities`
 * layer and would beat typeset's `components` layer rules.
 *
 * Two things the renderer does have to do, both from typeset's docs:
 * a wide table goes inside a `typeset-scroll` wrapper so it scrolls instead of
 * compressing, and the searchable/opting-out pieces (the code block) carry
 * `not-typeset` themselves.
 */
function NodeRenderer({ node }: { node: Content | Root }): React.ReactNode {
  switch (node.type) {
    case "heading": {
      const Tag = `h${node.depth}` as "h1" | "h2" | "h3" | "h4" | "h5" | "h6"
      return <Tag>{renderPhrasing(node)}</Tag>
    }

    case "paragraph":
      return <p>{renderPhrasing(node)}</p>

    case "strong":
      return <strong>{renderPhrasing(node)}</strong>

    case "emphasis":
      return <em>{renderPhrasing(node)}</em>

    case "delete":
      return <del>{renderPhrasing(node)}</del>

    case "inlineCode":
      return <code>{node.value}</code>

    case "link":
      return (
        <a href={node.url} target="_blank" rel="noopener noreferrer">
          {renderPhrasing(node)}
        </a>
      )

    case "image":
      return <img src={node.url} alt={node.alt || ""} />

    case "list": {
      const Tag = node.ordered ? "ol" : "ul"
      return (
        <Tag>
          {node.children.map((item, i) => (
            <li key={i}>{renderChildren(item)}</li>
          ))}
        </Tag>
      )
    }

    case "blockquote":
      return <blockquote>{renderChildren(node)}</blockquote>

    case "code":
      return (
        <CodeBlock language={node.lang || undefined}>
          {node.value}
        </CodeBlock>
      )

    case "table": {
      // GFM tables: row 0 is the header. Typeset styles `thead th` and
      // `tbody …` separately, so the rows are grouped that way.
      const [header, ...body] = node.children
      return (
        <div className="typeset-scroll">
          <table>
            <thead>
              <tr>
                {header?.children.map((cell, i) => (
                  <th key={i} align={node.align?.[i] ?? undefined}>
                    {renderPhrasing(cell)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {body.map((row, ri) => (
                <tr key={ri}>
                  {row.children.map((cell, ci) => (
                    <td key={ci} align={node.align?.[ci] ?? undefined}>
                      {renderPhrasing(cell)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    }

    case "thematicBreak":
      return <hr />

    case "html":
      return null

    case "text": {
      return node.value.replace(/\$\\\rightarrow\$/g, " → ").replace(/\\\rightarrow/g, " → ")
    }

    case "break":
      return <br />

    case "root":
      return renderChildren(node)

    default:
      return null
  }
}

export function renderMdast(nodes: (Content | Root)[]): React.ReactNode {
  return nodes.map((node, i) => <NodeRenderer key={i} node={node} />)
}
