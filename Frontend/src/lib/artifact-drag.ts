import type { ArtifactFile } from "@/lib/artifacts"

/**
 * Carrying a file from the artifacts rail to the composer.
 *
 * The drag rides on `dataTransfer`, but only as a label: what travels is the
 * listing row — a path, a name, a size, a modified stamp — never a byte of the
 * file, which lives on disk and is not the browser's to copy. The custom type is
 * how the composer tells a rail card apart from a file dragged in off the
 * desktop, which it has no way to read.
 */
export const ARTIFACT_DRAG_TYPE = "application/x-calabrass-artifact"

/** What a rail card puts on the drag. */
export function encodeArtifactDrag(file: ArtifactFile) {
  return JSON.stringify(file)
}

/** The file a drop carried, or null when the payload was anything else. */
export function decodeArtifactDrag(raw: string): ArtifactFile | null {
  try {
    const file = JSON.parse(raw) as Partial<ArtifactFile>
    if (typeof file?.path !== "string" || typeof file.name !== "string") {
      return null
    }
    // Filled in field by field: this came off a drag, so it is checked before
    // anything downstream treats it as a listing row.
    return {
      path: file.path,
      name: file.name,
      extension: typeof file.extension === "string" ? file.extension : "",
      size: typeof file.size === "number" ? file.size : 0,
      modified: typeof file.modified === "number" ? file.modified : 0,
    }
  } catch {
    return null
  }
}

/**
 * Whether the drag in flight is one of ours. `types` is all a drag is allowed to
 * show while it is still moving — the payload itself only becomes readable on
 * drop — which is enough to decide whether to light up the composer.
 */
export function isArtifactDrag(types: readonly string[]) {
  return types.includes(ARTIFACT_DRAG_TYPE)
}
