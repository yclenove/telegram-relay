/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** 后端 API 根地址，例如 http://127.0.0.1:8080 或留空表示同域 */
  readonly VITE_API_BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
