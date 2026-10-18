/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Absolute API root the desktop build points at. The web build leaves it
   * unset and talks to the same origin.
   */
  readonly VITE_API_BASE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
