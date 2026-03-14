"use server";

import { setSelectedOrgId } from "@/lib/org";

export async function switchOrganization(orgId: string) {
  await setSelectedOrgId(orgId);
}
