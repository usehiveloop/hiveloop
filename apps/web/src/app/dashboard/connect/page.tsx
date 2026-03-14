"use client";

import { useState } from "react";
import Link from "next/link";
import { Settings, Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";

type ThemeColors = {
  primary: string;
  background: string;
  text: string;
  border: string;
  error: string;
};

type Mode = "light" | "dark" | "system";

type Provider = {
  id: string;
  label: string;
  letter: string;
  color: string;
  domain: string;
  enabled: boolean;
};

const allProviders: Provider[] = [
  { id: "openai", label: "OpenAI", letter: "O", color: "#000000", domain: "api.openai.com", enabled: true },
  { id: "anthropic", label: "Anthropic", letter: "A", color: "#D4A574", domain: "api.anthropic.com", enabled: true },
  { id: "google", label: "Google AI", letter: "G", color: "#4285F4", domain: "generativelanguage.googleapis.com", enabled: true },
  { id: "azure", label: "Azure OpenAI", letter: "A", color: "#0078D4", domain: "*.openai.azure.com", enabled: false },
  { id: "mistral", label: "Mistral", letter: "M", color: "#EF4444", domain: "api.mistral.ai", enabled: false },
  { id: "cohere", label: "Cohere", letter: "C", color: "#10B981", domain: "api.cohere.ai", enabled: false },
  { id: "groq", label: "Groq", letter: "G", color: "#F97316", domain: "api.groq.com", enabled: true },
];

function ColorRow({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-[13px] text-foreground">{label}</span>
      <div className="flex items-center gap-2">
        <div className="size-6 border border-border" style={{ backgroundColor: value }} />
        <div className="border border-border bg-background">
          <Input
            value={value}
            onChange={(e) => onChange(e.target.value)}
            className="h-6.5 w-20 border-0 bg-transparent px-2 font-mono text-xs text-foreground"
          />
        </div>
      </div>
    </div>
  );
}

function ConfigureProvidersModal({
  open,
  onOpenChange,
  providers,
  onToggle,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providers: Provider[];
  onToggle: (id: string) => void;
}) {
  const [search, setSearch] = useState("");
  const enabledCount = providers.filter((p) => p.enabled).length;

  const filtered = providers.filter(
    (p) =>
      p.label.toLowerCase().includes(search.toLowerCase()) ||
      p.domain.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-160 flex-col gap-0 overflow-hidden p-0 sm:max-w-130" showCloseButton={false}>
        {/* Header */}
        <DialogHeader className="flex-row items-start justify-between space-y-0 border-b border-border px-6 pb-5 pt-6">
          <div className="flex flex-col gap-1">
            <DialogTitle className="font-mono text-lg font-semibold">Configure Providers</DialogTitle>
            <DialogDescription className="text-[13px]">
              Toggle which providers appear in the Connect widget.
            </DialogDescription>
          </div>
          <button onClick={() => onOpenChange(false)} className="text-dim hover:text-foreground">
            <X className="size-5" />
          </button>
        </DialogHeader>

        {/* Search */}
        <div className="px-6 pt-4">
          <div className="relative">
            <Search className="absolute left-3.5 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
            <Input
              placeholder={`Search ${providers.length} providers...`}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-9.5 bg-card pl-9 font-mono text-[13px]"
            />
          </div>
        </div>

        {/* Provider List */}
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-2">
          {filtered.map((provider) => (
            <div
              key={provider.id}
              className="flex items-center justify-between border-b border-border py-3 last:border-b-0"
            >
              <div className="flex items-center gap-3">
                <div
                  className="flex size-8 shrink-0 items-center justify-center rounded-md"
                  style={{ backgroundColor: provider.color }}
                >
                  <span className="text-sm font-semibold text-white">{provider.letter}</span>
                </div>
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-foreground">{provider.label}</span>
                  <span className="text-[11px] text-dim">{provider.domain}</span>
                </div>
              </div>
              <Switch
                checked={provider.enabled}
                onCheckedChange={() => onToggle(provider.id)}
              />
            </div>
          ))}
        </div>

        {/* Footer */}
        <div className="flex shrink-0 items-center justify-between border-t border-border px-6 py-4">
          <span className="text-[13px] text-dim">
            {enabledCount} of {providers.length} providers enabled
          </span>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export default function ConnectCustomizePage() {
  const [colors, setColors] = useState<ThemeColors>({
    primary: "#6D28D9",
    background: "#FFFFFF",
    text: "#1A1A1A",
    border: "#E5E7EB",
    error: "#EF4444",
  });
  const [borderRadius, setBorderRadius] = useState(8);
  const [fontFamily, setFontFamily] = useState("Inter");
  const [mode, setMode] = useState<Mode>("light");
  const [providers, setProviders] = useState(allProviders);
  const [providersModalOpen, setProvidersModalOpen] = useState(false);

  function updateColor(key: keyof ThemeColors, value: string) {
    setColors((prev) => ({ ...prev, [key]: value }));
  }

  function toggleProvider(id: string) {
    setProviders((prev) =>
      prev.map((p) => (p.id === id ? { ...p, enabled: !p.enabled } : p))
    );
  }

  const enabledProviders = providers.filter((p) => p.enabled);

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-2 border-b border-border px-4 py-4 sm:flex-row sm:items-start sm:justify-between sm:gap-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex flex-col gap-1">
          <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
            Connect UI
          </h1>
          <p className="text-sm text-dim">
            Customize how the Connect widget looks when embedded in your app.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Link href="/dashboard/connect/sessions">
            <Button variant="outline" size="lg">Sessions</Button>
          </Link>
          <Button variant="outline" size="lg">Integration Guide</Button>
          <Button size="lg">Save Theme</Button>
        </div>
      </header>

      {/* Content */}
      <div className="flex flex-1 flex-col overflow-hidden lg:flex-row">
        {/* Theme Controls */}
        <div className="flex shrink-0 flex-col gap-5 overflow-y-auto border-b border-border px-4 py-6 sm:px-6 lg:min-w-75 lg:w-[30%] lg:border-b-0 lg:border-r lg:px-6 lg:py-6">
          {/* Colors */}
          <div className="flex flex-col gap-3.5 border border-border bg-card px-5 py-5">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">Colors</span>
            <div className="flex flex-col gap-3">
              <ColorRow label="Primary" value={colors.primary} onChange={(v) => updateColor("primary", v)} />
              <ColorRow label="Background" value={colors.background} onChange={(v) => updateColor("background", v)} />
              <ColorRow label="Text" value={colors.text} onChange={(v) => updateColor("text", v)} />
              <ColorRow label="Border" value={colors.border} onChange={(v) => updateColor("border", v)} />
              <ColorRow label="Error" value={colors.error} onChange={(v) => updateColor("error", v)} />
            </div>
          </div>

          {/* Appearance */}
          <div className="flex flex-col gap-3.5 border border-border bg-card px-5 py-5">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">Appearance</span>
            <div className="flex flex-col gap-4">
              {/* Border Radius */}
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between">
                  <span className="text-[13px] text-foreground">Border Radius</span>
                  <span className="border border-border bg-background px-2 py-0.5 font-mono text-xs text-foreground">
                    {borderRadius}px
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-2xs text-dim">0</span>
                  <Slider
                    value={[borderRadius]}
                    onValueChange={(v) => setBorderRadius(Array.isArray(v) ? v[0] : v)}
                    min={0}
                    max={20}
                    className="flex-1"
                  />
                  <span className="text-2xs text-dim">20</span>
                </div>
              </div>

              {/* Font Family */}
              <div className="flex flex-col gap-2">
                <span className="text-[13px] text-foreground">Font Family</span>
                <Select value={fontFamily} onValueChange={(v) => v && setFontFamily(v)}>
                  <SelectTrigger className="h-8.5 w-full text-[13px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Inter">Inter</SelectItem>
                    <SelectItem value="IBM Plex Sans">IBM Plex Sans</SelectItem>
                    <SelectItem value="System">System Default</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Mode */}
              <div className="flex flex-col gap-2">
                <span className="text-[13px] text-foreground">Mode</span>
                <ToggleGroup
                  value={[mode]}
                  onValueChange={(v) => { if (v.length > 0) setMode(v[0] as Mode); }}
                  className="w-full rounded-none"
                >
                  {(["light", "dark", "system"] as Mode[]).map((m) => (
                    <ToggleGroupItem
                      key={m}
                      value={m}
                      className="flex-1 rounded-none py-2 text-[13px] data-pressed:bg-[#6D28D9] data-pressed:text-white"
                    >
                      {m === "light" ? "Light" : m === "dark" ? "Dark" : "System"}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </div>
            </div>
          </div>

          {/* Providers */}
          <div className="flex flex-col gap-3.5 border border-border bg-card px-5 py-5">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">Providers</span>
            <div className="flex flex-wrap gap-2">
              {enabledProviders.map((p) => (
                <div
                  key={p.id}
                  className="flex items-center gap-1.5 border border-primary/20 bg-primary/8 px-2.5 py-1"
                >
                  <div
                    className="flex size-4 items-center justify-center rounded-[3px]"
                    style={{ backgroundColor: p.color }}
                  >
                    <span className="text-[8px] font-semibold text-white">{p.letter}</span>
                  </div>
                  <span className="text-[13px] text-chart-2">{p.label}</span>
                </div>
              ))}
            </div>
            <Button
              variant="outline"
              className="gap-1.5 text-[13px]"
              onClick={() => setProvidersModalOpen(true)}
            >
              <Settings className="size-3.5" />
              Configure Providers
            </Button>
          </div>
        </div>

        {/* Preview Panel */}
        <div className="flex flex-1 flex-col overflow-hidden">
          <div className="flex items-center justify-between border-b border-border px-4 py-3 sm:px-6">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
              Live Preview
            </span>
            <button className="text-[13px] text-chart-2 hover:text-primary-foreground">
              Reset to defaults
            </button>
          </div>
          <div className="flex flex-1 items-center justify-center border border-border bg-surface p-10">
            {/* Blank preview canvas — will be populated with real data */}
          </div>
        </div>
      </div>

      {/* Configure Providers Modal */}
      <ConfigureProvidersModal
        open={providersModalOpen}
        onOpenChange={setProvidersModalOpen}
        providers={providers}
        onToggle={toggleProvider}
      />
    </>
  );
}
