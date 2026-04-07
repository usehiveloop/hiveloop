import { NextRequest, NextResponse } from "next/server"
import {
  getSession,
  createSessionCookie,
  createBackupSessionCookie,
  createImpersonatingCookie,
  type SessionData,
} from "@/lib/auth/session"

const API_URL = process.env.API_URL ?? (process.env.NEXT_PUBLIC_API_URL as string)

export async function POST(request: NextRequest) {
  const session = await getSession()
  if (!session) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 })
  }

  const body = await request.json()
  const userId = body.userId as string
  if (!userId) {
    return NextResponse.json({ error: "userId is required" }, { status: 400 })
  }

  const upstream = await fetch(`${API_URL}/admin/v1/users/${userId}/impersonate`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${session.access_token}`,
    },
  })

  if (!upstream.ok) {
    const error = await upstream.json().catch(() => ({ error: "impersonation failed" }))
    return NextResponse.json(error, { status: upstream.status })
  }

  const data = await upstream.json()
  if (!data.access_token || !data.refresh_token) {
    return NextResponse.json({ error: "invalid response from server" }, { status: 502 })
  }

  const impersonatedSession: SessionData = {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
  }

  const responseHeaders = new Headers()
  responseHeaders.append("set-cookie", await createBackupSessionCookie(session))
  responseHeaders.append("set-cookie", await createSessionCookie(impersonatedSession))
  responseHeaders.append(
    "set-cookie",
    createImpersonatingCookie({
      userId: data.user.id,
      email: data.user.email,
      name: data.user.name,
    })
  )

  const { access_token: _accessToken, refresh_token: _refreshToken, expires_in: _expiresIn, ...safe } = data

  return NextResponse.json(safe, { status: 200, headers: responseHeaders })
}
