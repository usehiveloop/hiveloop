import LogtoClient from "@logto/next/edge";
import { type NextRequest, NextResponse } from "next/server";
import { getLogtoConfig } from "@/lib/logto";

export async function middleware(request: NextRequest) {
  const client = new LogtoClient(getLogtoConfig());
  const { isAuthenticated } = await client.getLogtoContext(request);

  if (!isAuthenticated) {
    return NextResponse.redirect(new URL("/sign-in", request.url));
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*"],
};
