/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * The API's origin in production, e.g. https://plotting-society-api.onrender.com
   *
   * Left unset in development, where Vite proxies /api to localhost:8080 and the
   * browser stays on a single origin.
   */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
