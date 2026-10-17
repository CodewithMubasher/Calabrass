/**
 * The artifacts the rail shows, and how the detail view should render them.
 *
 * The type, `previewFor` and `languageFor` outlive the fake rows below: they are
 * what the real list will be fed into, and what decides whether an entry reads
 * as a document, as source, or as something we cannot show.
 */

export interface Artifact {
  id: string
  title: string
  kind: string
  /** Upper case, no dot: the list prints it as "Plan · MD". */
  extension: string
  /** The file body. Phase 2 reads it off disk; today the rows carry it. */
  content: string
}

export type ArtifactPreview = "markdown" | "code" | "none"

/** Extensions shiki has a grammar for. Anything missing is prose or opaque. */
const CODE_LANGUAGES: Record<string, string> = {
  TS: "typescript",
  TSX: "tsx",
  JS: "javascript",
  JSX: "jsx",
  JSON: "json",
  GO: "go",
  PY: "python",
  CSS: "css",
  HTML: "html",
  SH: "bash",
  YAML: "yaml",
  YML: "yaml",
  TOML: "toml",
  SQL: "sql",
  RS: "rust",
  TXT: "text",
}

/**
 * How the detail renders a file: prose goes through the markdown renderer (so
 * it reads exactly like a reply, typeset and all), source goes through the
 * shiki block, and anything else — a PDF, an image, an archive — has no preview
 * and says so instead of pretending.
 */
export function previewFor(extension: string): ArtifactPreview {
  const ext = extension.toUpperCase()
  if (ext === "MD" || ext === "MDX") return "markdown"
  return CODE_LANGUAGES[ext] !== undefined ? "code" : "none"
}

/** The shiki grammar for an extension, falling back to plain text. */
export function languageFor(extension: string): string {
  return CODE_LANGUAGES[extension.toUpperCase()] ?? "text"
}

const PRODUCT_REQUIREMENTS = `# Product requirements

**Status:** draft · **Owner:** design · **Target:** v0.4

Calabrass is a local-first AI workspace. This document is the contract for the
first release: what ships, what waits, and how we will know it worked.

## Goals

1. One window that holds conversations, artifacts and voice.
2. Nothing leaves the machine unless the user asks for it.
3. Every long-running action is visible while it runs.

## Scope

| Area | Ships in v0.4 | Waits |
| --- | --- | --- |
| Chat | streaming, traces, per-chat context | branching |
| Artifacts | list, detail, upload | versioning |
| Voice | one exchange at a time | barge-in |

> Drafts are cheap. Shipping is not. Cut scope, not quality.

## Open questions

- Do artifacts belong to a chat, or to the project?
- Do we index file *contents*, or only their metadata?

## Where it plugs in

\`\`\`ts
const artifact = await artifacts.open(id)
render(artifact.content, { typeset: true })
\`\`\`

Everything below this line is placeholder, kept so the viewer has something
worth scrolling.

## Notes

The rail is deliberately quiet. It is a place documents rest, not a page that
competes with the conversation, so the only weight in it comes from the
documents themselves.
`

const RELEASE_CHECKLIST = `# Release checklist

What has to be true before v0.4 goes out. Order matters — each step assumes the
one above it passed.

## Before the cut

1. Freeze the tool schema and read every description once more.
2. Run the harness on both providers and write down what failed.
3. Re-read the artifacts spec end to end, and delete what it no longer asks for.

## Manual pass

| Check | Owner | Notes |
| --- | --- | --- |
| New workspace creates and deletes | backend | files must survive the delete |
| Trace collapses and expands inline | frontend | no layout shift while streaming |
| Voice survives a pause and resume | frontend | one exchange on screen at a time |

## After the cut

- Tag the release and write the notes from the diff, not from memory.
- Keep the provider keys out of the notes, the screenshots and the logs.
`

const AUTH_MIDDLEWARE = `import { createMiddleware } from "./pipeline"

/**
 * Resolves the caller before the handler runs. A missing or malformed token is
 * a 401 and never a 500: a client should be able to tell our bugs apart from
 * its own.
 */
export const authenticate = createMiddleware(async (req, next) => {
  const header = req.headers.get("authorization")
  const token = header?.startsWith("Bearer ") ? header.slice(7) : null

  if (!token) {
    return new Response("unauthorized", { status: 401 })
  }

  const claims = await verify(token)
  if (!claims) {
    return new Response("unauthorized", { status: 401 })
  }

  return next({ ...req, ctx: { ...req.ctx, userId: claims.sub } })
})
`

/**
 * Fake rows, here only to shape the list and exercise the three renderings.
 * The real ones come from the artifacts store in phase 2; id, title, kind,
 * extension and content are exactly what that API is expected to return.
 */
export const PLACEHOLDER_ARTIFACTS: Artifact[] = [
  {
    id: "prd",
    title: "Product requirements",
    kind: "Document",
    extension: "MD",
    content: PRODUCT_REQUIREMENTS,
  },
  {
    id: "plan",
    title: "Release checklist",
    kind: "Plan",
    extension: "MD",
    content: RELEASE_CHECKLIST,
  },
  {
    id: "auth",
    title: "Auth middleware",
    kind: "Code",
    extension: "TS",
    content: AUTH_MIDDLEWARE,
  },
  {
    id: "style",
    title: "Style reference",
    kind: "Image",
    // Binary, so there is nothing to preview and nothing to copy.
    extension: "PNG",
    content: "",
  },
]
