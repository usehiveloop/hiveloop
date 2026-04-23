import { cookies } from "next/headers"
import { log } from "@/lib/logger"

const COOKIE_NAME = "__session"
const BACKUP_COOKIE_NAME = "__session_backup"
const IMPERSONATING_COOKIE_NAME = "__impersonating"
const MAX_AGE = 60 * 60 * 24 * 30 // 30 days

export type SessionData = {
  access_token: string
  refresh_token: string
  expires_at: number // Unix ms when access_token expires
}

let cachedKey: CryptoKey | null = null

async function getKey(): Promise<CryptoKey> {
  if (cachedKey) return cachedKey

  const secret = process.env.SESSION_SECRET
  if (!secret) {
    log.error("SESSION_SECRET env var is not set")
    throw new Error("SESSION_SECRET env var is required")
  }

  log.debug("deriving session encryption key")
  const raw = new TextEncoder().encode(secret)
  const base = await crypto.subtle.importKey("raw", raw, "HKDF", false, [
    "deriveKey",
  ])

  cachedKey = await crypto.subtle.deriveKey(
    { name: "HKDF", hash: "SHA-256", salt: new Uint8Array(32), info: new TextEncoder().encode("session") },
    base,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"]
  )

  log.info("session encryption key derived successfully")
  return cachedKey
}

export async function encrypt(data: SessionData): Promise<string> {
  log.debug({ expires_at: data.expires_at }, "encrypting session data")
  const key = await getKey()
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const plaintext = new TextEncoder().encode(JSON.stringify(data))

  const ciphertext = new Uint8Array(
    await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, plaintext)
  )

  // iv (12) + ciphertext+tag
  const buf = new Uint8Array(iv.length + ciphertext.length)
  buf.set(iv)
  buf.set(ciphertext, iv.length)

  const encoded = btoa(String.fromCharCode(...buf))
  log.debug({ cookie_length: encoded.length }, "session encrypted")
  return encoded
}

export async function decrypt(cookie: string): Promise<SessionData | null> {
  try {
    const key = await getKey()
    const buf = Uint8Array.from(atob(cookie), (c) => c.charCodeAt(0))

    const iv = buf.slice(0, 12)
    const ciphertext = buf.slice(12)

    const plaintext = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv },
      key,
      ciphertext
    )

    log.debug("session decrypted successfully")
    return JSON.parse(new TextDecoder().decode(plaintext)) as SessionData
  } catch (err) {
    log.warn({ err }, "session decrypt failed")
    return null
  }
}

export async function getSession(): Promise<SessionData | null> {
  const store = await cookies()
  const cookie = store.get(COOKIE_NAME)
  if (!cookie?.value) return null
  return decrypt(cookie.value)
}

export async function createSessionCookie(data: SessionData): Promise<string> {
  const value = await encrypt(data)
  const secure = process.env.NODE_ENV === "production" ? "; Secure" : ""
  return `${COOKIE_NAME}=${value}; HttpOnly; SameSite=Lax; Path=/; Max-Age=${MAX_AGE}${secure}`
}

export function clearSessionCookie(): string {
  return `${COOKIE_NAME}=; HttpOnly; SameSite=Lax; Path=/; Max-Age=0`
}

export function getSessionCookieName(): string {
  return COOKIE_NAME
}

/**
 * Read __session from a raw cookie header string (for use in the proxy
 * where we have the request but not the Next.js cookies() API).
 */
export async function getSessionFromHeader(
  cookieHeader: string | null
): Promise<SessionData | null> {
  if (!cookieHeader) return null
  const match = cookieHeader
    .split(";")
    .map((c) => c.trim())
    .find((c) => c.startsWith(`${COOKIE_NAME}=`))
  if (!match) return null
  const value = match.slice(COOKIE_NAME.length + 1)
  return decrypt(value)
}

/**
 * Strip the __session cookie from a cookie header string so it isn't
 * forwarded to the Go backend.
 */
export function stripSessionCookie(cookieHeader: string): string {
  return cookieHeader
    .split(";")
    .map((c) => c.trim())
    .filter((c) => !c.startsWith(`${COOKIE_NAME}=`))
    .join("; ")
}

export type ImpersonatingInfo = {
  userId: string
  email: string
  name: string
}

export async function createBackupSessionCookie(data: SessionData): Promise<string> {
  const value = await encrypt(data)
  const secure = process.env.NODE_ENV === "production" ? "; Secure" : ""
  return `${BACKUP_COOKIE_NAME}=${value}; HttpOnly; SameSite=Lax; Path=/; Max-Age=${MAX_AGE}${secure}`
}

export async function getBackupSession(): Promise<SessionData | null> {
  const store = await cookies()
  const cookie = store.get(BACKUP_COOKIE_NAME)
  if (!cookie?.value) return null
  return decrypt(cookie.value)
}

export function clearBackupSessionCookie(): string {
  return `${BACKUP_COOKIE_NAME}=; HttpOnly; SameSite=Lax; Path=/; Max-Age=0`
}

export function createImpersonatingCookie(info: ImpersonatingInfo): string {
  const value = encodeURIComponent(JSON.stringify(info))
  const secure = process.env.NODE_ENV === "production" ? "; Secure" : ""
  return `${IMPERSONATING_COOKIE_NAME}=${value}; SameSite=Lax; Path=/; Max-Age=${MAX_AGE}${secure}`
}

export function clearImpersonatingCookie(): string {
  return `${IMPERSONATING_COOKIE_NAME}=; SameSite=Lax; Path=/; Max-Age=0`
}

export function getImpersonatingCookieName(): string {
  return IMPERSONATING_COOKIE_NAME
}
