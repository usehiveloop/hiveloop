import { signOut } from "@logto/next/server-actions";
import { getLogtoConfig } from "@/lib/logto";

export async function GET() {
  await signOut(getLogtoConfig());
}
