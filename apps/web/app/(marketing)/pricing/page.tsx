import { PricingClient } from "./pricing-client"
import type { components } from "@/lib/api/schema"

export const dynamic = "force-static"

type Plan = components["schemas"]["planDTO"]

function plansURL() {
  const base =
    process.env.HIVY_API_URL ?? process.env.NEXT_PUBLIC_HIVY_API_URL ?? ""
  if (!base) {
    throw new Error(
      "Pricing page static build requires HIVY_API_URL or NEXT_PUBLIC_HIVY_API_URL",
    )
  }
  return new URL("/v1/plans", base).toString()
}

async function getPlans(): Promise<Plan[]> {
  const response = await fetch(plansURL(), {
    cache: "force-cache",
  })

  if (!response.ok) {
    throw new Error(
      `Failed to fetch pricing plans during static build: ${response.status} ${response.statusText}`,
    )
  }

  return (await response.json()) as Plan[]
}

export default async function PricingPage() {
  const plans = await getPlans()
  return <PricingClient plans={plans} />
}
