import { NextResponse } from "next/server";
import { getAccessToken, getRefreshToken, clearAuthCookies } from "@/lib/auth";
import { apiLogout } from "@/lib/auth-api";

export async function POST() {
  try {
    const accessToken = await getAccessToken();
    const refreshToken = await getRefreshToken();

    if (accessToken && refreshToken) {
      await apiLogout(accessToken, refreshToken);
    }

    await clearAuthCookies();

    return NextResponse.json({ success: true });
  } catch {
    // Clear cookies even if the backend call fails
    await clearAuthCookies();
    return NextResponse.json({ success: true });
  }
}
