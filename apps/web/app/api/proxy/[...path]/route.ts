import { NextRequest, NextResponse } from "next/server"
import {
  getSessionFromHeader,
  stripSessionCookie,
  createSessionCookie,
  clearSessionCookie,
  type SessionData,
} from "@/lib/auth/session"
import { log } from "@/lib/logger"

const API_URL = process.env.API_URL ?? process.env.NEXT_PUBLIC_API_URL as string

log.info({ api_url: API_URL }, "proxy route initialized")

// Paths whose successful responses contain tokens that should be persisted.
const AUTH_PATHS = new Set([
  "oauth/exchange",
  "auth/login",
  "auth/register",
  "auth/refresh",
  "auth/otp/verify",
])

const LOGOUT_PATH = "auth/logout"

// ---------------------------------------------------------------------------
// Concurrent refresh lock
// ---------------------------------------------------------------------------

let refreshPromise: Promise<SessionData | null> | null = null

async function refreshTokens(refreshToken: string): Promise<SessionData | null> {
  const res = await fetch(`${API_URL}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })

  if (!res.ok) return null

  const data = await res.json()
  if (!data.access_token || !data.refresh_token) return null

  return {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
  }
}

async function safeRefresh(refreshToken: string): Promise<SessionData | null> {
  if (refreshPromise) return refreshPromise
  refreshPromise = refreshTokens(refreshToken).finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

// ---------------------------------------------------------------------------
// Build headers for upstream request
// ---------------------------------------------------------------------------

function buildUpstreamHeaders(
  req: NextRequest,
  session: SessionData | null
): Headers {
  const headers = new Headers()

  // Forward safe headers (no authorization — we inject it from the cookie)
  for (const key of ["content-type", "accept"]) {
    const value = req.headers.get(key)
    if (value) headers.set(key, value)
  }

  // Forward cookies minus __session
  const rawCookies = req.headers.get("cookie")
  if (rawCookies) {
    const cleaned = stripSessionCookie(rawCookies)
    if (cleaned) headers.set("cookie", cleaned)
  }

  // Inject auth from session
  if (session) {
    headers.set("authorization", `Bearer ${session.access_token}`)
  }

  // Inject active org from cookie
  const activeOrgCookie = req.cookies.get("hiveloop_active_org")
  if (activeOrgCookie?.value) {
    headers.set("X-Org-ID", activeOrgCookie.value)
  }

  return headers
}

// ---------------------------------------------------------------------------
// Forward a request to the Go backend
// ---------------------------------------------------------------------------

async function forward(
  url: URL,
  method: string,
  headers: Headers,
  body: ArrayBuffer | undefined
) {
  return fetch(url, { method, headers, body })
}

// ---------------------------------------------------------------------------
// Main handler
// ---------------------------------------------------------------------------

async function handler(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params
  const apiPath = path.join("/")
  const reqLog = log.child({ method: req.method, path: apiPath })

  reqLog.info("proxy request started")

  const url = new URL(`${API_URL}/${apiPath}`)
  req.nextUrl.searchParams.forEach((value, key) =>
    url.searchParams.append(key, value)
  )

  reqLog.debug({ upstream_url: url.toString() }, "upstream url resolved")

  const rawCookies = req.headers.get("cookie")
  let session = await getSessionFromHeader(rawCookies)
  reqLog.debug({ has_session: !!session }, "session check")

  const body =
    req.method !== "GET" && req.method !== "HEAD"
      ? await req.arrayBuffer()
      : undefined

  // -----------------------------------------------------------------------
  // Logout interception: inject refresh_token into request body
  // -----------------------------------------------------------------------
  let upstreamBody: ArrayBuffer | undefined = body
  const isLogout = apiPath === LOGOUT_PATH && req.method === "POST"

  if (isLogout && session) {
    const payload = body ? JSON.parse(new TextDecoder().decode(body)) : {}
    payload.refresh_token = session.refresh_token
    upstreamBody = new TextEncoder().encode(JSON.stringify(payload)).buffer as ArrayBuffer
  }

  // -----------------------------------------------------------------------
  // Proactive refresh if token is about to expire
  // -----------------------------------------------------------------------
  if (session && !AUTH_PATHS.has(apiPath) && session.expires_at - Date.now() < 60_000) {
    const refreshed = await safeRefresh(session.refresh_token)
    if (refreshed) session = refreshed
  }

  // -----------------------------------------------------------------------
  // Forward to backend
  // -----------------------------------------------------------------------
  const headers = buildUpstreamHeaders(req, session)
  let upstream: Response
  try {
    upstream = await forward(url, req.method, headers, upstreamBody)
    reqLog.info({ status: upstream.status }, "upstream response received")
  } catch (err) {
    reqLog.error({ err, upstream_url: url.toString() }, "upstream fetch failed")
    return NextResponse.json({ error: "upstream_unavailable" }, { status: 502 })
  }

  // -----------------------------------------------------------------------
  // Auto-refresh on 401 (retry once)
  // -----------------------------------------------------------------------
  let refreshedSession: SessionData | null = null

  if (upstream.status === 401 && session && !AUTH_PATHS.has(apiPath) && !isLogout) {
    reqLog.info("got 401, attempting token refresh")
    const newSession = await safeRefresh(session.refresh_token)
    if (newSession) {
      reqLog.info("token refresh succeeded, retrying request")
      refreshedSession = newSession
      session = newSession
      const retryHeaders = buildUpstreamHeaders(req, newSession)
      upstream = await forward(url, req.method, retryHeaders, body)
      reqLog.info({ status: upstream.status }, "retry response received")
    } else {
      reqLog.warn("token refresh failed")
    }
  }

  // -----------------------------------------------------------------------
  // Build response
  // -----------------------------------------------------------------------
  const responseHeaders = new Headers()
  const skipHeaders = new Set(["transfer-encoding", "content-encoding", "content-length"])
  upstream.headers.forEach((value, key) => {
    if (skipHeaders.has(key.toLowerCase())) return
    responseHeaders.set(key, value)
  })

  // -----------------------------------------------------------------------
  // Intercept auth responses — persist session, strip tokens from body
  // -----------------------------------------------------------------------
  if (AUTH_PATHS.has(apiPath) && upstream.ok) {
    reqLog.info("intercepting auth response")
    try {
      const data = await upstream.json()
      reqLog.debug({ has_access_token: !!data.access_token, has_refresh_token: !!data.refresh_token }, "auth response parsed")

      if (data.access_token && data.refresh_token) {
        const newSession: SessionData = {
          access_token: data.access_token,
          refresh_token: data.refresh_token,
          expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
        }
        const cookie = await createSessionCookie(newSession)
        reqLog.debug({ cookie_length: cookie.length }, "session cookie created")

        // Build clean response headers — don't carry over content-length
        // from the upstream response since we're returning a different body
        const authHeaders = new Headers()
        authHeaders.append("set-cookie", cookie)

        // Strip tokens from what the client receives
        const { access_token: _a, refresh_token: _r, expires_in: _e, ...safe } = data
        reqLog.info({ response_keys: Object.keys(safe), status: upstream.status }, "auth response complete, session cookie set")
        return NextResponse.json(safe, {
          status: upstream.status,
          headers: authHeaders,
        })
      }
    } catch (err) {
      reqLog.error({ err }, "auth response interception failed")
      return NextResponse.json({ error: "session_creation_failed" }, { status: 502 })
    }
  }

  // Attach updated session cookie if we refreshed mid-request
  if (refreshedSession) {
    responseHeaders.append(
      "set-cookie",
      await createSessionCookie(refreshedSession)
    )
  }

  // Proactive refresh — also persist the updated cookie
  if (
    !AUTH_PATHS.has(apiPath) &&
    !refreshedSession &&
    session &&
    rawCookies
  ) {
    const original = await getSessionFromHeader(rawCookies)
    if (original && original.access_token !== session.access_token) {
      responseHeaders.append(
        "set-cookie",
        await createSessionCookie(session)
      )
    }
  }

  // Logout — clear session cookie
  if (isLogout && upstream.ok) {
    responseHeaders.append("set-cookie", clearSessionCookie())
  }

  // Clear session on auth failure when refresh also failed
  if (upstream.status === 401 && session && !refreshedSession && !AUTH_PATHS.has(apiPath)) {
    responseHeaders.append("set-cookie", clearSessionCookie())
  }

  reqLog.info({ status: upstream.status }, "proxy response complete")
  return new NextResponse(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  })
}

export const GET = handler
export const POST = handler
export const PUT = handler
export const PATCH = handler
export const DELETE = handler
