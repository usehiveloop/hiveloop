"use client"

import * as React from "react"
import cronstrue from "cronstrue"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

type Mode = "hourly" | "daily" | "weekdays" | "weekly" | "custom"

const HOUR_INTERVALS = [1, 2, 3, 4, 6, 8, 12] as const

// Cron weekday: 0 = Sun, 1 = Mon, ..., 6 = Sat
const DAYS = [
  { label: "S", value: "0", full: "Sunday" },
  { label: "M", value: "1", full: "Monday" },
  { label: "T", value: "2", full: "Tuesday" },
  { label: "W", value: "3", full: "Wednesday" },
  { label: "T", value: "4", full: "Thursday" },
  { label: "F", value: "5", full: "Friday" },
  { label: "S", value: "6", full: "Saturday" },
] as const

function timeToParts(time: string): { hour: number; minute: number } {
  const [h, m] = time.split(":")
  return {
    hour: Number.parseInt(h ?? "0", 10),
    minute: Number.parseInt(m ?? "0", 10),
  }
}

function buildCron(state: {
  mode: Mode
  hourlyEvery: number
  hourlyMinute: number
  time: string
  weeklyDays: string[]
  custom: string
}): string {
  const { hour, minute } = timeToParts(state.time)
  switch (state.mode) {
    case "hourly":
      return state.hourlyEvery === 1
        ? `${state.hourlyMinute} * * * *`
        : `${state.hourlyMinute} */${state.hourlyEvery} * * *`
    case "daily":
      return `${minute} ${hour} * * *`
    case "weekdays":
      return `${minute} ${hour} * * 1-5`
    case "weekly": {
      const days = [...state.weeklyDays]
        .map((d) => Number.parseInt(d, 10))
        .sort((a, b) => a - b)
        .join(",")
      return `${minute} ${hour} * * ${days || "1"}`
    }
    case "custom":
      return state.custom
  }
}

function describe(cron: string): { ok: boolean; text: string } {
  if (!cron.trim()) return { ok: false, text: "Enter a cron expression." }
  try {
    return { ok: true, text: cronstrue.toString(cron, { use24HourTimeFormat: true }) }
  } catch (error) {
    return {
      ok: false,
      text: error instanceof Error ? error.message : "Invalid cron expression.",
    }
  }
}

interface CronScheduleInputProps {
  value: string
  onChange: (cron: string) => void
}

export function CronScheduleInput({ value, onChange }: CronScheduleInputProps) {
  const [mode, setMode] = React.useState<Mode>("daily")
  const [hourlyEvery, setHourlyEvery] = React.useState(1)
  const [hourlyMinute, setHourlyMinute] = React.useState(0)
  const [time, setTime] = React.useState("09:00")
  const [weeklyDays, setWeeklyDays] = React.useState<string[]>(["1"])
  const [custom, setCustom] = React.useState(value || "")

  React.useEffect(() => {
    const next = buildCron({
      mode,
      hourlyEvery,
      hourlyMinute,
      time,
      weeklyDays,
      custom,
    })
    if (next !== value) onChange(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, hourlyEvery, hourlyMinute, time, weeklyDays, custom])

  const generated = buildCron({
    mode,
    hourlyEvery,
    hourlyMinute,
    time,
    weeklyDays,
    custom,
  })
  const description = describe(generated)

  return (
    <div className="flex flex-col gap-3">
      <Tabs value={mode} onValueChange={(v) => setMode(v as Mode)}>
        <TabsList className="w-full">
          <TabsTrigger value="hourly">Hourly</TabsTrigger>
          <TabsTrigger value="daily">Daily</TabsTrigger>
          <TabsTrigger value="weekdays">Weekdays</TabsTrigger>
          <TabsTrigger value="weekly">Weekly</TabsTrigger>
          <TabsTrigger value="custom">Custom</TabsTrigger>
        </TabsList>

        {/* Hourly */}
        <TabsContent value="hourly" className="mt-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[13px] text-muted-foreground">Every</span>
            <Select
              value={String(hourlyEvery)}
              onValueChange={(v) => setHourlyEvery(Number.parseInt(v, 10))}
            >
              <SelectTrigger className="h-9 w-20">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {HOUR_INTERVALS.map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {n}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <span className="text-[13px] text-muted-foreground">
              hour{hourlyEvery !== 1 ? "s" : ""}, at minute
            </span>
            <Input
              type="number"
              min={0}
              max={59}
              value={hourlyMinute}
              onChange={(e) =>
                setHourlyMinute(
                  Math.max(0, Math.min(59, Number.parseInt(e.target.value || "0", 10)))
                )
              }
              className="h-9 w-20"
            />
          </div>
        </TabsContent>

        {/* Daily */}
        <TabsContent value="daily" className="mt-4">
          <div className="flex items-center gap-2">
            <span className="text-[13px] text-muted-foreground">Every day at</span>
            <Input
              type="time"
              value={time}
              onChange={(e) => setTime(e.target.value)}
              className="h-9 w-32 font-mono"
            />
          </div>
        </TabsContent>

        {/* Weekdays */}
        <TabsContent value="weekdays" className="mt-4">
          <div className="flex items-center gap-2">
            <span className="text-[13px] text-muted-foreground">Mon–Fri at</span>
            <Input
              type="time"
              value={time}
              onChange={(e) => setTime(e.target.value)}
              className="h-9 w-32 font-mono"
            />
          </div>
        </TabsContent>

        {/* Weekly */}
        <TabsContent value="weekly" className="mt-4 flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label className="text-[12px] text-muted-foreground">Days</Label>
            <ToggleGroup
              multiple
              value={weeklyDays}
              onValueChange={(next) => setWeeklyDays(next as string[])}
              variant="outline"
              spacing={4}
            >
              {DAYS.map((day) => (
                <ToggleGroupItem
                  key={day.value}
                  value={day.value}
                  aria-label={day.full}
                  className="size-8"
                >
                  {day.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-[13px] text-muted-foreground">at</span>
            <Input
              type="time"
              value={time}
              onChange={(e) => setTime(e.target.value)}
              className="h-9 w-32 font-mono"
            />
          </div>
        </TabsContent>

        {/* Custom */}
        <TabsContent value="custom" className="mt-4 flex flex-col gap-2">
          <Input
            value={custom}
            onChange={(e) => setCustom(e.target.value)}
            placeholder="0 9 * * *"
            className="font-mono"
            autoFocus
          />
          <p className="text-[11px] text-muted-foreground">
            5-field syntax:{" "}
            <code className="font-mono text-[11px]">minute hour day month weekday</code>.
          </p>
        </TabsContent>
      </Tabs>

      <div className="flex flex-col gap-1 rounded-md border border-border/60 bg-muted/30 px-3 py-2">
        <p
          className={
            "text-[12px] " +
            (description.ok ? "text-foreground" : "text-destructive")
          }
        >
          {description.text}
        </p>
        {description.ok ? (
          <p className="font-mono text-[11px] text-muted-foreground">{generated}</p>
        ) : null}
      </div>
    </div>
  )
}
