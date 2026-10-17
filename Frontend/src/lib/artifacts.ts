/**
 * The files a workspace holds, and how the rail reads them.
 *
 * `ArtifactFile` is what the backend lists — the listing is a live read of the
 * workspace folder, so there is no local copy of it and no id to invent: a
 * file's relative path is its identity. Everything below is the viewer's own
 * business: how a name reads in a list, and whether a file is prose, source, or
 * something we can only hand over.
 */

/** One file in a workspace, as the artifacts stream lists it. */
export interface ArtifactFile {
  /** Relative to the workspace, with forward slashes. */
  path: string
  /** The file's own name, extension included. */
  name: string
  /** Upper case, no dot. Empty when the file has no extension. */
  extension: string
  /** Bytes on disk. */
  size: number
  /** Unix milliseconds, which is how the list is ordered. */
  modified: number
}

export type ArtifactPreview = "markdown" | "code" | "none"

/** Extensions shiki has a grammar for. */
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

const DOCUMENT_EXTENSIONS = new Set(["MD", "MDX", "TXT", "PDF", "DOC", "DOCX", "RTF", "ODT"])
const IMAGE_EXTENSIONS = new Set(["PNG", "JPG", "JPEG", "GIF", "WEBP", "SVG", "ICO", "BMP", "AVIF"])
const ARCHIVE_EXTENSIONS = new Set(["ZIP", "TAR", "GZ", "TGZ", "7Z", "RAR", "BZ2", "XZ"])
const DATA_EXTENSIONS = new Set(["CSV", "TSV", "XML", "LOG"])

/**
 * How the detail renders a file: prose goes through the markdown renderer (so
 * it reads exactly like a reply, typeset and all), source goes through the
 * shiki block, and anything else is handed over as a download instead. The
 * backend has the last word — a file that turns out to be binary is shown the
 * same way as an unknown extension.
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

/** The word the list line leads with, before the extension. */
export function kindFor(extension: string): string {
  const ext = extension.toUpperCase()
  if (ext === "") return "File"
  if (DOCUMENT_EXTENSIONS.has(ext)) return "Document"
  if (IMAGE_EXTENSIONS.has(ext)) return "Image"
  if (ARCHIVE_EXTENSIONS.has(ext)) return "Archive"
  if (DATA_EXTENSIONS.has(ext)) return "Data"
  return CODE_LANGUAGES[ext] !== undefined ? "Code" : "File"
}

/**
 * How a file's name reads in the rail. The extension is already on the line
 * below it, and dashes and underscores are how a filename joins words, not how
 * a person does.
 */
export function titleFrom(name: string): string {
  const stem = name.replace(/\.[^./\\]+$/, "")
  const words = stem.replace(/[-_]+/g, " ").trim()
  if (words === "") return name
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/**
 * The folder a file sits in, rendered the way it should be shown before the
 * name: "docs/", or nothing at all for a file at the top of the workspace.
 */
export function directoryOf(file: ArtifactFile): string {
  const prefix = file.path.slice(0, file.path.length - file.name.length)
  return prefix
}
