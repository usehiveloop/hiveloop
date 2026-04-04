import { cookies } from "next/headers"

const COOKIE_NAME = "__session"
const MAX_AGE = 60 * 60 * 24 * 30 // 30 days

export type SessionData = {
  access_token: string
  refresh_token: string
  expires_at: number // Unix ms when access_token expires
}

// ---------------------------------------------------------------------------
// Key derivation — HKDF from SESSION_SECRET → AES-256-GCM key
// ---------------------------------------------------------------------------

let cachedKey: CryptoKey | null = null

async function getKey(): Promise<CryptoKey> {
  if (cachedKey) return cachedKey

  const secret = process.env.SESSION_SECRET
  if (!secret) throw new Error("SESSION_SECRET env var is required")

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

  return cachedKey
}

// ---------------------------------------------------------------------------
// Encrypt / Decrypt
// ---------------------------------------------------------------------------

export async function encrypt(data: SessionData): Promise<string> {
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

  return btoa(String.fromCharCode(...buf))
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

    return JSON.parse(new TextDecoder().decode(plaintext)) as SessionData
  } catch {
    return null
  }
}

// ---------------------------------------------------------------------------
// Cookie helpers
// ---------------------------------------------------------------------------

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
