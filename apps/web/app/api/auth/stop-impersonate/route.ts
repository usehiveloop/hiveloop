import { NextResponse } from "next/server"
import {
  getBackupSession,
  createSessionCookie,
  clearBackupSessionCookie,
  clearImpersonatingCookie,
} from "@/lib/auth/session"

export async function POST() {
  const backupSession = await getBackupSession()
  if (!backupSession) {
    return NextResponse.json({ error: "not impersonating" }, { status: 400 })
  }

  const responseHeaders = new Headers()
  responseHeaders.append("set-cookie", await createSessionCookie(backupSession))
  responseHeaders.append("set-cookie", clearBackupSessionCookie())
  responseHeaders.append("set-cookie", clearImpersonatingCookie())

  return NextResponse.json({ ok: true }, { status: 200, headers: responseHeaders })
}
