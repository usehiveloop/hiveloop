"use client"

import { useMemo } from "react"
import { Controller, useWatch } from "react-hook-form"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ImagePicker } from "@/components/image-picker"
import { CategoryCombobox } from "@/app/w/agents/new/_components/category-combobox"
import { $api } from "@/lib/api/hooks"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

export function EmployeeStep() {
  const { form, goNext } = useOnboarding()

  const { data: categoriesData, isLoading: categoriesLoading } = $api.useQuery(
    "get",
    "/v1/agents/categories",
  )
  const categories = useMemo(
    () =>
      (categoriesData ?? []).map((c) => ({
        id: c.id ?? "",
        name: c.name ?? c.id ?? "",
        description: c.description ?? undefined,
      })),
    [categoriesData],
  )

  const watchedName = useWatch({ control: form.control, name: "agentName" })
  const watchedCategory = useWatch({ control: form.control, name: "agentCategory" })
  const canContinue = (watchedName?.trim().length ?? 0) > 0 && watchedCategory.length > 0

  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-10">
      <StepHeader
        title="Meet your AI employee"
        description="Give them a name, a face, and a job. You can refine all of this later."
      />

      <div className="flex flex-col gap-6">
        <Controller
          control={form.control}
          name="agentAvatarUrl"
          render={({ field }) => (
            <section className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <Label className="text-[13px] font-medium">Avatar</Label>
                <p className="mt-0.5 text-[12px] text-muted-foreground">
                  Square. PNG, JPEG, WEBP, or GIF. Up to 5 MB.
                </p>
              </div>
              <ImagePicker
                assetType="avatar"
                value={field.value || undefined}
                onChange={(url) => field.onChange(url ?? "")}
                fallback={watchedName?.[0]?.toUpperCase() ?? "?"}
                ariaLabel={field.value ? "Replace avatar" : "Upload avatar"}
              />
            </section>
          )}
        />

        <div className="flex flex-col gap-2.5">
          <Label htmlFor="employee-name">Name</Label>
          <Input
            id="employee-name"
            placeholder="e.g. Haraki"
            autoComplete="off"
            spellCheck={false}
            {...form.register("agentName")}
          />
        </div>

        <div className="flex flex-col gap-2.5">
          <Label htmlFor="employee-description">
            Description{" "}
            <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Textarea
            id="employee-description"
            placeholder="Handles incoming customer questions, escalates billing issues, and follows up on missed replies."
            rows={4}
            {...form.register("agentDescription")}
          />
        </div>

        <Controller
          control={form.control}
          name="agentCategory"
          render={({ field }) => (
            <div className="flex flex-col gap-2.5">
              <Label htmlFor="employee-category">Category</Label>
              <CategoryCombobox
                categories={categories}
                loading={categoriesLoading}
                value={field.value || undefined}
                onSelect={field.onChange}
              />
            </div>
          )}
        />
      </div>

      <Button
        onClick={goNext}
        disabled={!canContinue}
        className="flex h-12 w-full items-center justify-center gap-2"
      >
        Next
        <HugeiconsIcon icon={ArrowRight01Icon} className="size-4" />
      </Button>
    </div>
  )
}
