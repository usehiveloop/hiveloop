import process from "node:process"
import type { ComponentType } from "react"
import { createElement } from "react"
import { render } from "@react-email/render"
import { Resend } from "resend"

import type { TemplateDefinition } from "../templates"

process.env.HIVY_EMAIL_RENDER_TARGET = "resend"

const dryRun = process.argv.includes("--dry-run")
const apiKey = process.env.RESEND_API_KEY ?? process.env.HIVY_RESEND_API_KEY

type ResendTemplate = {
  id: string
  object: "template"
}

type ResendError = {
  message?: string
  name?: string
  statusCode?: number | null
}

type ResendResult<T> = {
  data: T | null
  error: ResendError | null
}

async function main() {
  if (!apiKey && !dryRun) {
    throw new Error("Set RESEND_API_KEY or HIVY_RESEND_API_KEY before uploading templates.")
  }

  const { templateRegistry } = await import("../registry")
  const resend = dryRun ? null : new Resend(apiKey)

  for (const entry of templateRegistry) {
    const html = await render(createElement(entry.component as ComponentType<any>, entry.placeholderProps), {
      pretty: true,
    })

    if (dryRun) {
      const variables = entry.definition.variables.map((variable) => variable.key).join(", ")
      console.log(`would upload ${entry.definition.alias} (${entry.definition.variables.length} variables: ${variables})`)
      continue
    }

    await upsertTemplate(resend, entry.definition, html)
  }
}

async function upsertTemplate(resend: Resend | null, definition: TemplateDefinition, html: string) {
  if (!resend) {
    throw new Error("Resend client is required outside dry-run mode.")
  }

  const existing = await getTemplate(resend, definition.alias)
  if (!existing) {
    const created = await sdkCall(() =>
      resend.templates.create({
        ...templatePayload(definition, html),
        alias: definition.alias,
      }),
    )
    await publishTemplate(resend, created.id)
    console.log(`created and published ${definition.alias}`)
    return
  }

  await sdkCall(() => resend.templates.update(existing.id, templatePayload(definition, html)))
  await publishTemplate(resend, existing.id)
  console.log(`updated and published ${definition.alias}`)
}

async function getTemplate(resend: Resend, alias: string): Promise<ResendTemplate | null> {
  const result = await sdkResult(() => resend.templates.get(alias))
  if (result.error && isNotFound(result.error)) return null
  return unwrap(result, `get template ${alias}`)
}

async function publishTemplate(resend: Resend, id: string) {
  await sdkCall(() => resend.templates.publish(id))
}

async function sdkCall<T>(fn: () => PromiseLike<ResendResult<T>>): Promise<T> {
  return unwrap(await sdkResult(fn), "resend template request")
}

async function sdkResult<T>(
  fn: () => PromiseLike<ResendResult<T>>,
  attempts = 3,
): Promise<ResendResult<T>> {
  let lastErr: unknown
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      return await fn()
    } catch (err) {
      lastErr = err
      if (attempt === attempts) break
      await new Promise((resolve) => setTimeout(resolve, attempt * 500))
    }
  }
  throw lastErr
}

function unwrap<T>(result: ResendResult<T>, action: string): T {
  if (result.error) {
    const message = result.error.message ?? result.error.name ?? "unknown Resend error"
    throw new Error(`${action}: ${message}`)
  }
  if (!result.data) {
    throw new Error(`${action}: empty Resend response`)
  }
  return result.data
}

function isNotFound(error: ResendError): boolean {
  return error.statusCode === 404 || /not found/i.test(error.message ?? "")
}

function templatePayload(definition: TemplateDefinition, html: string) {
  return {
    name: definition.name,
    subject: definition.subject,
    html,
    variables: definition.variables,
  }
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
