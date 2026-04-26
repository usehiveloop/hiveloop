"use client"

import { useState, useMemo } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon, ArrowLeft01Icon, ArrowRight01Icon, Cancel01Icon, ArrowDown01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { ProviderLogo } from "@/components/provider-logo"
import { cn } from "@/lib/utils"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"

interface AddLlmKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated?: (credentialId: string) => void
}

export function AddLlmKeyDialog({ open, onOpenChange, onCreated }: AddLlmKeyDialogProps) {
  const queryClient = useQueryClient()
  const [selectedProvider, setSelectedProvider] = useState<{ id: string; name: string; api?: string } | null>(null)
  const [search, setSearch] = useState("")
  const [label, setLabel] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [baseUrl, setBaseUrl] = useState("")
  const [showAdvanced, setShowAdvanced] = useState(false)
  const createCredential = $api.useMutation("post", "/v1/credentials")

  const { data, isLoading } = $api.useQuery(
    "get",
    "/v1/providers",
    {},
    { enabled: open },
  )

  const providers = data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return providers
    const query = search.toLowerCase()
    return providers.filter(
      (provider) =>
        (provider.name ?? "").toLowerCase().includes(query) ||
        (provider.id ?? "").toLowerCase().includes(query),
    )
  }, [providers, search])

  function selectProvider(provider: { id?: string; name?: string; api?: string }) {
    setSelectedProvider({ id: provider.id ?? "", name: provider.name ?? "", api: provider.api })
    setLabel("")
    setApiKey("")
    setBaseUrl(provider.api ?? "")
    setShowAdvanced(false)
  }

  function goBack() {
    setSelectedProvider(null)
  }

  function reset() {
    setSelectedProvider(null)
    setSearch("")
    setLabel("")
    setApiKey("")
    setBaseUrl("")
    setShowAdvanced(false)
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!selectedProvider || !apiKey.trim()) return

    createCredential.mutate(
      {
        body: {
          provider_id: selectedProvider.id,
          api_key: apiKey.trim(),
          label: label.trim() || `${selectedProvider.name} key`,
          base_url: baseUrl.trim() || undefined,
        } as never,
      },
      {
        onSuccess: (data) => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/credentials"] })
          toast.success(`${selectedProvider.name} key added`)
          const credentialId = (data as { id?: string })?.id
          reset()
          onOpenChange(false)
          if (credentialId) onCreated?.(credentialId)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to save credential"))
        },
      },
    )
  }

  const innerVariants = {
    enter: { opacity: 0 },
    center: { opacity: 1 },
    exit: { opacity: 0 },
  }

  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) reset()
        onOpenChange(nextOpen)
      }}
    >
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/50 data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0" />
        <DialogPrimitive.Popup
        className={cn(
          "fixed top-1/2 left-1/2 z-50 w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-4xl bg-popover p-6 text-sm text-popover-foreground shadow-xl ring-1 ring-foreground/5 duration-100 outline-none sm:max-w-md h-[600px] dark:ring-foreground/10",
          "data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
        )}
      >
        <AnimatePresence mode="wait">
          {selectedProvider ? (
            <motion.div
              key="form"
              variants={innerVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15 }}
              className="h-full"
            >
              <div className="flex flex-col gap-1.5 mb-6">
                <div className="flex items-center gap-2">
                  <button type="button" onClick={goBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
                    <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
                  </button>
                  <div className="flex items-center gap-2.5">
                    <ProviderLogo provider={selectedProvider.id} size={20} />
                    <DialogPrimitive.Title className="font-heading text-base leading-none font-medium">
                      {selectedProvider.name}
                    </DialogPrimitive.Title>
                  </div>
                </div>
                <DialogPrimitive.Description className="text-sm text-muted-foreground">
                  Enter your API key to connect {selectedProvider.name}.
                </DialogPrimitive.Description>
              </div>

              <form onSubmit={handleSubmit} autoComplete="off" className="flex flex-col gap-5">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="llm-label">Label</Label>
                  <Input
                    id="llm-label"
                    name="llm-label-nofill"
                    autoComplete="off"
                    value={label}
                    onChange={(event) => setLabel(event.target.value)}
                    placeholder={`${selectedProvider.name} key`}
                  />
                </div>

                <div className="flex flex-col gap-2">
                  <Label htmlFor="llm-api-key">API key</Label>
                  <Input
                    id="llm-api-key"
                    name="llm-api-key-nofill"
                    type="password"
                    autoComplete="new-password"
                    value={apiKey}
                    onChange={(event) => setApiKey(event.target.value)}
                    placeholder="sk-..."
                    required
                    autoFocus
                  />
                </div>

                <button
                  type="button"
                  onClick={() => setShowAdvanced((prev) => !prev)}
                  className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                >
                  <HugeiconsIcon
                    icon={ArrowDown01Icon}
                    size={12}
                    className={cn("transition-transform", showAdvanced && "rotate-180")}
                  />
                  Advanced options
                </button>

                <AnimatePresence initial={false}>
                  {showAdvanced && (
                    <motion.div
                      initial={{ height: 0, opacity: 0 }}
                      animate={{ height: "auto", opacity: 1 }}
                      exit={{ height: 0, opacity: 0 }}
                      transition={{ duration: 0.2, ease: "easeInOut" as const }}
                      className="overflow-hidden"
                    >
                      <div className="flex flex-col gap-2">
                        <Label htmlFor="llm-base-url">
                          Base URL <span className="text-muted-foreground font-normal">(optional)</span>
                        </Label>
                        <Input
                          id="llm-base-url"
                          value={baseUrl}
                          onChange={(event) => setBaseUrl(event.target.value)}
                          placeholder={selectedProvider.api ?? "https://api.example.com/v1"}
                        />
                      </div>
                    </motion.div>
                  )}
                </AnimatePresence>

                <Button type="submit" className="w-full" loading={createCredential.isPending} disabled={!apiKey.trim()}>
                  Save credential
                </Button>
              </form>
            </motion.div>
          ) : (
            <motion.div
              key="list"
              variants={innerVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15 }}
              className="h-full"
            >
              <div className="flex flex-col gap-1.5 mb-6">
                <DialogPrimitive.Title className="font-heading text-base leading-none font-medium">
                  Add llm key
                </DialogPrimitive.Title>
                <DialogPrimitive.Description className="text-sm text-muted-foreground">
                  Choose a provider and enter your API key to connect.
                </DialogPrimitive.Description>
              </div>

              <div className="relative mb-4">
                <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search providers..."
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  className="pl-9 h-9"
                  autoFocus
                />
              </div>

              <ScrollArea className="h-100">
                {isLoading ? (
                  <div className="flex flex-col gap-2">
                    {Array.from({ length: 6 }).map((_, index) => (
                      <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
                    ))}
                  </div>
                ) : filtered.length === 0 ? (
                  <div className="flex items-center justify-center h-full">
                    <p className="text-sm text-muted-foreground">No providers found.</p>
                  </div>
                ) : (
                  <div className="flex flex-col gap-1">
                    {filtered.map((provider) => (
                      <button
                        key={provider.id}
                        type="button"
                        className="flex items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors hover:bg-muted cursor-pointer"
                        onClick={() => selectProvider(provider)}
                      >
                        <ProviderLogo provider={provider.id ?? ""} size={28} />
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium truncate">{provider.name}</p>
                          <p className="text-xs text-muted-foreground truncate">
                            {provider.model_count} models
                          </p>
                        </div>
                        <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0" />
                      </button>
                    ))}
                  </div>
                )}
              </ScrollArea>
            </motion.div>
          )}
        </AnimatePresence>

        <DialogPrimitive.Close
          render={
            <Button
              variant="ghost"
              className="absolute top-4 right-4 bg-secondary"
              size="icon-sm"
            />
          }
        >
          <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
          <span className="sr-only">Close</span>
        </DialogPrimitive.Close>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
