import { createServer } from "node:http"
import { readFileSync } from "node:fs"

const catalog = JSON.parse(readFileSync("global/plans/catalog.json", "utf8"))
const plans = catalog.plans
  .filter((plan) => plan.active && plan.visible)
  .sort((a, b) => a.price_cents - b.price_cents || a.slug.localeCompare(b.slug))
  .map((plan) => ({
    slug: plan.slug,
    name: plan.name,
    provider: plan.provider,
    monthly_credits: plan.monthly_credits,
    welcome_credits: plan.welcome_credits,
    price_cents: plan.price_cents,
    currency: plan.currency,
    features: plan.features,
  }))

const server = createServer((request, response) => {
  if (request.method === "GET" && request.url === "/v1/plans") {
    response.writeHead(200, { "content-type": "application/json" })
    response.end(JSON.stringify(plans))
    return
  }

  response.writeHead(404, { "content-type": "application/json" })
  response.end(JSON.stringify({ error: "not found" }))
})

server.listen(18081, "127.0.0.1", () => {
  console.log("ci web plans api listening on 127.0.0.1:18081")
})
