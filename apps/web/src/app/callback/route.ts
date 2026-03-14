import { handleSignIn } from "@logto/next/server-actions";
import { redirect } from "next/navigation";
import { NextRequest } from "next/server";
import { getLogtoConfig } from "@/lib/logto";

export async function GET(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  await handleSignIn(getLogtoConfig(), searchParams);
  redirect("/dashboard");
}
