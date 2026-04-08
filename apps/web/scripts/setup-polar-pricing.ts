import { Polar } from "@polar-sh/sdk"
import type { Meter } from "@polar-sh/sdk/models/components/meter.js"
import type { Benefit } from "@polar-sh/sdk/models/components/benefit.js"
import type { Product } from "@polar-sh/sdk/models/components/product.js"
import type { MeterCreate } from "@polar-sh/sdk/models/components/metercreate.js"

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

interface MeterConfig {
  name: string
  filter: MeterCreate["filter"]
  aggregation: MeterCreate["aggregation"]
}

interface BenefitConfig {
  description: string
  meterName: string
  units: number
  rollover: boolean
}

interface PriceConfig {
  amountType: "free" | "fixed" | "metered_unit"
  priceAmount?: number
  priceCurrency?: "usd"
  meterName?: string
  unitAmount?: number
}

interface ProductConfig {
  name: string
  description: string
  prices: PriceConfig[]
  benefitDescriptions: string[]
}

const METERS: MeterConfig[] = [
  {
    name: "shared_agent_runs",
    filter: {
      conjunction: "and",
      clauses: [
        { property: "name", operator: "eq", value: "agent_run" },
        { property: "sandbox_type", operator: "eq", value: "shared" },
      ],
    },
    aggregation: { func: "count" },
  },
  {
    name: "dedicated_agent_runs",
    filter: {
      conjunction: "and",
      clauses: [
        { property: "name", operator: "eq", value: "agent_run" },
        { property: "sandbox_type", operator: "eq", value: "dedicated" },
      ],
    },
    aggregation: { func: "count" },
  },
]

const BENEFITS: BenefitConfig[] = [
  {
    description: "Free: 100 shared runs/month",
    meterName: "shared_agent_runs",
    units: 100,
    rollover: false,
  },
  {
    description: "Pro: 300 shared runs/month",
    meterName: "shared_agent_runs",
    units: 300,
    rollover: false,
  },
  {
    description: "Pro: 300 dedicated runs/month",
    meterName: "dedicated_agent_runs",
    units: 300,
    rollover: false,
  },
]

const PRODUCTS: ProductConfig[] = [
  {
    name: "Free",
    description: "1 agent, 100 runs/month. For exploring and prototyping.",
    prices: [{ amountType: "free" }],
    benefitDescriptions: ["Free: 100 shared runs/month"],
  },
  {
    name: "Pro Shared",
    description:
      "$4.99/agent/month. 300 runs included, $0.01/run overage. Shared sandbox.",
    prices: [
      { amountType: "fixed", priceAmount: 499, priceCurrency: "usd" },
      { amountType: "metered_unit", meterName: "shared_agent_runs", unitAmount: 1 },
    ],
    benefitDescriptions: ["Pro: 300 shared runs/month"],
  },
  {
    name: "Pro Dedicated",
    description:
      "$6.99/agent/month. 300 runs included, $0.05/run overage. Dedicated sandbox with full system access.",
    prices: [
      { amountType: "fixed", priceAmount: 699, priceCurrency: "usd" },
      { amountType: "metered_unit", meterName: "dedicated_agent_runs", unitAmount: 5 },
    ],
    benefitDescriptions: ["Pro: 300 dedicated runs/month"],
  },
]

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function log(action: string, resource: string, name: string, resourceId: string) {
  console.log(`  ${action.padEnd(10)} ${resource.padEnd(10)} ${name.padEnd(40)} ${resourceId}`)
}

async function findOrCreateMeter(
  polar: Polar,
  config: MeterConfig,
  organizationId: string | undefined,
): Promise<Meter> {
  const listResponse = await polar.meters.list({
    query: config.name,
    limit: 100,
    ...(organizationId ? { organizationId } : {}),
  })

  const existing = listResponse.result.items.find(
    (meter) => meter.name === config.name,
  )

  if (existing) {
    log("exists", "meter", config.name, existing.id)
    return existing
  }

  const created = await polar.meters.create({
    name: config.name,
    filter: config.filter,
    aggregation: config.aggregation,
    ...(organizationId ? { organizationId } : {}),
  })

  log("created", "meter", config.name, created.id)
  return created
}

async function findOrCreateBenefit(
  polar: Polar,
  config: BenefitConfig,
  meterId: string,
  organizationId: string | undefined,
): Promise<Benefit> {
  const listResponse = await polar.benefits.list({
    query: config.description,
    typeFilter: "meter_credit",
    limit: 100,
    ...(organizationId ? { organizationId } : {}),
  })

  const existing = listResponse.result.items.find(
    (benefit) => benefit.description === config.description,
  )

  if (existing) {
    log("exists", "benefit", config.description, existing.id)
    return existing
  }

  const created = await polar.benefits.create({
    type: "meter_credit",
    description: config.description,
    properties: {
      meterId,
      units: config.units,
      rollover: config.rollover,
    },
    ...(organizationId ? { organizationId } : {}),
  })

  log("created", "benefit", config.description, created.id)
  return created
}

async function findOrCreateProduct(
  polar: Polar,
  config: ProductConfig,
  meterIdsByName: Map<string, string>,
  benefitIdsByDescription: Map<string, string>,
  organizationId: string | undefined,
): Promise<Product> {
  const listResponse = await polar.products.list({
    query: config.name,
    isRecurring: true,
    limit: 100,
    ...(organizationId ? { organizationId } : {}),
  })

  const existing = listResponse.result.items.find(
    (product) => product.name === config.name,
  )

  if (existing) {
    log("exists", "product", config.name, existing.id)
    return existing
  }

  const resolvedPrices = config.prices.map((price) => {
    if (price.amountType === "free") {
      return { amountType: "free" as const }
    }

    if (price.amountType === "fixed") {
      return {
        amountType: "fixed" as const,
        priceAmount: price.priceAmount!,
        priceCurrency: price.priceCurrency,
      }
    }

    const meterId = meterIdsByName.get(price.meterName!)
    if (!meterId) {
      throw new Error(`Meter not found for price: ${price.meterName}`)
    }

    return {
      amountType: "metered_unit" as const,
      meterId,
      unitAmount: price.unitAmount!,
    }
  })

  const created = await polar.products.create({
    name: config.name,
    description: config.description,
    recurringInterval: "month",
    prices: resolvedPrices,
    ...(organizationId ? { organizationId } : {}),
  })

  log("created", "product", config.name, created.id)

  const benefitIds = config.benefitDescriptions
    .map((description) => benefitIdsByDescription.get(description))
    .filter((benefitId): benefitId is string => benefitId !== undefined)

  if (benefitIds.length > 0) {
    await polar.products.updateBenefits({
      id: created.id,
      productBenefitsUpdate: { benefits: benefitIds },
    })
    console.log(`             attached ${benefitIds.length} benefit(s) to ${config.name}`)
  }

  return created
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const accessToken = process.env.POLAR_ACCESS_TOKEN
  if (!accessToken) {
    console.error("Error: POLAR_ACCESS_TOKEN environment variable is required")
    process.exit(1)
  }

  const organizationId = process.env.POLAR_ORGANIZATION_ID || undefined
  const server = (process.env.POLAR_SERVER || "sandbox") as "sandbox" | "production"

  console.log(`\nPolar pricing setup (${server})\n`)
  console.log("=".repeat(80))

  const polar = new Polar({ accessToken, server })

  // Phase 1: Meters (parallel)
  console.log("\n[Phase 1] Meters\n")
  const meters = await Promise.all(
    METERS.map((config) => findOrCreateMeter(polar, config, organizationId)),
  )

  const meterIdsByName = new Map<string, string>()
  for (const meter of meters) {
    meterIdsByName.set(meter.name, meter.id)
  }

  // Phase 2: Benefits (parallel, depends on meter IDs)
  console.log("\n[Phase 2] Benefits\n")
  const benefits = await Promise.all(
    BENEFITS.map((config) => {
      const meterId = meterIdsByName.get(config.meterName)
      if (!meterId) {
        throw new Error(`Meter not found: ${config.meterName}`)
      }
      return findOrCreateBenefit(polar, config, meterId, organizationId)
    }),
  )

  const benefitIdsByDescription = new Map<string, string>()
  for (const benefit of benefits) {
    benefitIdsByDescription.set(benefit.description, benefit.id)
  }

  // Phase 3: Products (sequential — each needs benefit attachment)
  console.log("\n[Phase 3] Products\n")
  const products: Product[] = []
  for (const config of PRODUCTS) {
    const product = await findOrCreateProduct(
      polar,
      config,
      meterIdsByName,
      benefitIdsByDescription,
      organizationId,
    )
    products.push(product)
  }

  // Summary
  console.log("\n" + "=".repeat(80))
  console.log("\nSetup complete. Summary:\n")

  console.log("Meters:")
  for (const meter of meters) {
    console.log(`  - ${meter.name}: ${meter.id}`)
  }

  console.log("\nBenefits:")
  for (const benefit of benefits) {
    console.log(`  - ${benefit.description}: ${benefit.id}`)
  }

  console.log("\nProducts:")
  for (const product of products) {
    console.log(`  - ${product.name}: ${product.id}`)
  }

  console.log("")
}

main().catch((error) => {
  console.error("\nFatal error:", error)
  process.exit(1)
})
