"use client"

import { useEffect, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"
import { PageHeader } from "@/components/page-header"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  ArrowUp02Icon,
  Loading03Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

export default function WorkspaceHome() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [draft, setDraft] = useState("")
  const [agentId, setAgentId] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

  useEffect(() => {
    if (searchParams.get("checkout") === "success") {
      toast.success("Subscription activated! You're on the Pro plan.")
      router.replace("/w")
    }
  }, [searchParams, router])

  const { data: employeesData, isLoading: employeesLoading } = $api.useQuery(
    "get",
    "/v1/employees",
  )
  const employees = employeesData?.data ?? []

  useEffect(() => {
    if (!agentId && employees.length > 0 && employees[0]?.id) {
      setAgentId(employees[0].id)
    }
  }, [agentId, employees])

  const selected = employees.find((e) => e.id === agentId) ?? null

  const createChat = $api.useMutation("post", "/v1/employees/{id}/chats")

  function handleSend() {
    if (!agentId || !draft.trim() || createChat.isPending) return
    createChat.mutate(
      {
        params: { path: { id: agentId } },
        body: { message: draft.trim() },
      },
      {
        onSuccess: (data) => {
          if (!data.session_id || !data.stream_url) {
            toast.error("Could not start chat — missing session id")
            return
          }
          const url = `/w/chats/${data.session_id}?stream=${encodeURIComponent(data.stream_url)}`
          router.push(url)
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Could not start chat"))
        },
      },
    )
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      handleSend()
    }
  }

  const canSend = Boolean(agentId) && draft.trim().length > 0 && !createChat.isPending

  return (
    <>
      <PageHeader title="Home" />
      <div className="mx-auto w-full max-w-3xl px-6 pt-16 pb-24">
        <div className="mb-8">
          <h1 className="font-heading text-[28px] font-medium leading-tight tracking-tight text-foreground">
            What do you want shipped first?
          </h1>
          <p className="mt-2 text-[14px] text-muted-foreground">
            Pick one of your AI employees, give them a task, watch them work.
          </p>
        </div>

        <div className="rounded-2xl border border-border bg-background transition-colors focus-within:border-foreground/30">
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={
              selected
                ? `Ask ${selected.name ?? "your employee"} for help…`
                : employeesLoading
                  ? "Loading employees…"
                  : "No employees yet — create one from /onboarding."
            }
            disabled={employeesLoading || !selected}
            className="block h-[150px] w-full resize-none bg-transparent px-4 pt-4 text-[14.5px] text-foreground outline-none placeholder:text-muted-foreground/70 disabled:opacity-50"
          />
          <div className="flex items-center justify-between gap-2 px-3 pt-1 pb-3">
            <Popover open={pickerOpen} onOpenChange={setPickerOpen}>
              <PopoverTrigger
                render={
                  <button
                    type="button"
                    disabled={employees.length === 0}
                    className="flex items-center gap-2 rounded-full border border-border/70 py-1 pl-1 pr-3 text-[12.5px] text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground disabled:opacity-50"
                  >
                    <Avatar className="h-6 w-6">
                      {selected?.avatar_url ? (
                        <AvatarImage src={selected.avatar_url} alt="" />
                      ) : null}
                      <AvatarFallback className="text-[10px]">
                        {(selected?.name ?? "?")[0]?.toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <span className="font-medium text-foreground">
                      {selected?.name ?? "Select employee"}
                    </span>
                    <HugeiconsIcon
                      icon={ArrowDown01Icon}
                      size={12}
                      className="opacity-60"
                    />
                  </button>
                }
              />
              <PopoverContent align="start" className="w-[340px] p-1.5">
                <div className="px-2.5 pt-2 pb-1.5 text-[10.5px] font-medium uppercase tracking-wide text-muted-foreground">
                  Choose an employee
                </div>
                <div className="space-y-0.5">
                  {employees.map((e) => {
                    if (!e.id) return null
                    const isActive = e.id === agentId
                    return (
                      <button
                        key={e.id}
                        type="button"
                        onClick={() => {
                          setAgentId(e.id ?? null)
                          setPickerOpen(false)
                        }}
                        className={`flex w-full items-start gap-3 rounded-lg px-2.5 py-2.5 text-left transition-colors ${
                          isActive ? "bg-muted/60" : "hover:bg-muted/40"
                        }`}
                      >
                        <Avatar className="h-9 w-9">
                          {e.avatar_url ? (
                            <AvatarImage src={e.avatar_url} alt="" />
                          ) : null}
                          <AvatarFallback>
                            {(e.name ?? "?")[0]?.toUpperCase()}
                          </AvatarFallback>
                        </Avatar>
                        <div className="min-w-0 flex-1">
                          <div className="text-[13.5px] font-medium text-foreground">
                            {e.name}
                          </div>
                          {e.description ? (
                            <div className="mt-0.5 truncate text-[11.5px] leading-relaxed text-muted-foreground">
                              {e.description}
                            </div>
                          ) : null}
                        </div>
                        <HugeiconsIcon
                          icon={Tick02Icon}
                          size={13}
                          className={`mt-1 shrink-0 text-primary ${isActive ? "" : "opacity-0"}`}
                        />
                      </button>
                    )
                  })}
                </div>
              </PopoverContent>
            </Popover>
            <button
              type="button"
              onClick={handleSend}
              disabled={!canSend}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-30"
            >
              {createChat.isPending ? (
                <HugeiconsIcon
                  icon={Loading03Icon}
                  size={14}
                  className="animate-spin"
                />
              ) : (
                <HugeiconsIcon icon={ArrowUp02Icon} size={16} />
              )}
            </button>
          </div>
        </div>
      </div>
    </>
  )
}
