import { previewFor, type ArtifactFile } from "@/lib/artifacts"

/**
 * What the composer knows about a file someone attached.
 *
 * Nothing is uploaded and nothing is sent anywhere: a file is read in the
 * browser, only far enough to describe it in a card. `lines` is absent for
 * anything that is not text, which is the card's cue to fall back on the size.
 */
export interface AttachmentFile {
  id: string
  /** The file's own name, extension included. */
  name: string
  /** Upper case, no dot. Empty when the name has none. */
  extension: string
  /** Bytes in the file. */
  size: number
  /** How many lines it holds, once it has been read. */
  lines?: number
}

/** Past this a file is described by its size rather than read. */
const READ_LIMIT = 2 * 1024 * 1024

/** The upper-case extension of a name, or "". */
function extensionOf(name: string) {
  const match = /\.[^./\\]+$/.exec(name)
  return match ? match[0].slice(1).toUpperCase() : ""
}

/**
 * Describes a picked file for the composer. It is read here rather than in the
 * card so the card stays a plain description of something it was handed, and
 * only the length is taken from the contents: no bytes are kept.
 */
export async function attachmentFromFile(
  file: File,
  id: string
): Promise<AttachmentFile> {
  const name = file.name || "Untitled"
  const extension = extensionOf(name)
  const attachment: AttachmentFile = { id, name, extension, size: file.size }

  if (previewFor(extension) === "none" || file.size > READ_LIMIT) {
    return attachment
  }

  try {
    const text = await file.text()
    if (text !== "") {
      // A closing newline ends the last line, it does not start another one.
      attachment.lines = text.replace(/\r?\n$/, "").split(/\r\n|\r|\n/).length
    } else {
      attachment.lines = 0
    }
  } catch {
    // Not readable as text after all; the size is what we have to go on.
  }

  return attachment
}

/**
 * Describes a file dragged out of a workspace. The rail already knows what the
 * listing knows — name, type, size — so a drop is described the moment it lands,
 * with nothing read: the file is on disk, not in the browser, and a card only
 * needs to say what it is. The length, when it is known at all, comes from the
 * other route in: a file the browser itself was handed.
 */
export function attachmentFromArtifact(
  file: ArtifactFile,
  id: string
): AttachmentFile {
  return { id, name: file.name, extension: file.extension, size: file.size }
}
