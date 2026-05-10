"use client"

import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, CpuIcon } from "@hugeicons/core-free-icons"
import { motion } from "motion/react"

export default function Home() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      <div className="flex flex-1 px-4 sm:px-6 lg:px-8">
        <div className="w-full max-w-424 lg:min-h-325 mx-auto relative overflow-hidden">
          {/* Grid background */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 80%)",
            }}
          />
          {/* Hero glow */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 40%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)",
            }}
          />
          <div className="relative flex flex-col items-center gap-6 sm:gap-8 pt-12 sm:pt-16 lg:pt-25 px-6 sm:px-8 lg:px-10">
            <div className="flex items-center gap-2 px-4 py-2 bg-muted border border-border rounded-full">
              <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
              <span className="font-mono text-[11px] font-medium uppercase tracking-[0.5px] text-muted-foreground">
                Hire AI employees for your team
              </span>
            </div>
            <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-[56px] font-bold text-foreground text-center leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
              The AI employee for any role
            </h1>
            <p className="text-base sm:text-lg lg:text-xl text-muted-foreground text-center leading-relaxed max-w-160">
              Hire AI employees that learn your ways, work autonomously,
              <br className="hidden sm:block" />
              and take initiative on their own.
            </p>
            <div className="flex flex-col sm:flex-row gap-2.5 pt-2 w-full sm:w-auto">
              <Input
                type="email"
                placeholder="Enter your email"
                className="h-10 sm:h-12 sm:w-72 rounded-full text-sm sm:text-base px-5"
              />
              <Link href="/demo" className="sm:hidden">
                <Button size="default" className="rounded-full h-10 w-full">Book a Demo</Button>
              </Link>
              <Link href="/demo" className="hidden sm:inline-block">
                <Button size="lg" className="rounded-full h-12">Book a Demo</Button>
              </Link>
            </div>
          </div>

          <div className="px-6 lg:px-10">
            <div className="relative z-10 w-full max-w-5xl bg-black dark:bg-card min-h-60 sm:min-h-80 lg:min-h-180 mt-8 sm:mt-12 lg:mt-16 mx-auto border border-border rounded-4xl shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] flex items-center justify-center">
              <div className="relative flex items-center justify-center">
                {/* Pulse rings */}
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full border border-foreground/20 lg:border-2 animate-[ping_2.5s_ease-out_infinite]" />
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full border border-foreground/12 lg:border-2 animate-[ping_2.5s_ease-out_0.8s_infinite]" />
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full bg-foreground/5 animate-[ping_2.5s_ease-out_0.4s_infinite]" />
                <svg
                  className="relative w-8 h-8 lg:w-20 lg:h-20 text-muted-foreground"
                  viewBox="0 0 24 24"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <title>play</title>
                  <path
                    fillRule="evenodd"
                    clipRule="evenodd"
                    d="M7.23832 3.04445C5.65196 2.1818 3.75 3.31957 3.75 5.03299L3.75 18.9672C3.75 20.6806 5.65196 21.8184 7.23832 20.9557L20.0503 13.9886C21.6499 13.1188 21.6499 10.8814 20.0503 10.0116L7.23832 3.04445ZM2.25 5.03299C2.25 2.12798 5.41674 0.346438 7.95491 1.72669L20.7669 8.6938C23.411 10.1317 23.411 13.8685 20.7669 15.3064L7.95491 22.2735C5.41674 23.6537 2.25 21.8722 2.25 18.9672L2.25 5.03299Z"
                    fill="currentColor"
                  />
                </svg>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Value proposition: AI employees */}
      <motion.section
        initial={{ opacity: 0, y: 20 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, margin: "-100px" }}
        transition={{ duration: 0.6, ease: "easeOut" }}
        className="w-full px-6 sm:px-8 lg:px-10"
      >
        <div className="w-full max-w-424 mx-auto relative">
          {/* Grid background */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 70%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
            {/* Section header */}
            <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
              <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                Why Hiveloop
              </p>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
                AI employees that learn, understand, take initiative.
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                Unlike generic AI tools, Hiveloop employees learn your ways of working,
                understand your organization, and take initiative on their own.
              </p>
            </div>

            {/* AI Employees Grid */}
            <div className="w-full max-w-5xl mx-auto rounded-4xl border border-border ring-1 ring-foreground/5 shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] overflow-hidden bg-background">
              <div className="p-6 sm:p-8 lg:p-10">
                <div className="flex items-center gap-2 mb-8">
                  <span className="w-2 h-2 rounded-full bg-primary" />
                  <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-foreground">
                    AI Employees
                  </span>
                </div>

                <div className="flex flex-col gap-3">
                  {[
                    {
                      name: "code-reviewer",
                      desc: "Reviews PRs on every push",
                    },
                    {
                      name: "ui-builder",
                      desc: "Generates components from designs",
                    },
                    {
                      name: "content-writer",
                      desc: "Drafts blog posts from outlines",
                    },
                    {
                      name: "support-agent",
                      desc: "Answers tickets from your docs",
                    },
                    {
                      name: "code-assistant",
                      desc: "Helps across the codebase",
                    },
                    {
                      name: "deploy-monitor",
                      desc: "Watches deploys, alerts on failure",
                    },
                  ].map((agent) => (
                    <div
                      key={agent.name}
                      className="flex items-center justify-between py-3 px-4 rounded-2xl bg-muted/40 dark:bg-white/[0.04] border border-border/60"
                    >
                      <div className="flex items-center gap-3">
                        <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
                        <div className="flex flex-col">
                          <span className="text-sm font-semibold font-mono text-foreground">
                            {agent.name}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {agent.desc}
                          </span>
                        </div>
                      </div>
                      <span className="text-xs font-mono text-green-600 dark:text-green-400 bg-green-500/10 px-2 py-0.5 rounded-full">
                        active
                      </span>
                    </div>
                  ))}
                </div>

                <div className="mt-8 flex flex-col items-center gap-2">
                  <span className="font-mono text-[11px] text-muted-foreground uppercase tracking-[1.5px]">
                    Hire your first AI employee
                  </span>
                  <Link href="/demo">
                    <Button variant="outline" size="sm" className="rounded-full">
                      Book a Demo
                      <HugeiconsIcon
                        icon={ArrowRight01Icon}
                        size={14}
                        className="ml-1 opacity-80"
                      />
                    </Button>
                  </Link>
                </div>
              </div>
            </div>

            {/* Stats */}
            <div className="grid grid-cols-3 gap-6 sm:gap-12 lg:gap-20 pt-4 sm:pt-8 px-4">
              {[
                {
                  value: "Learning",
                  label: "adapts to your ways",
                },
                {
                  value: "Autonomous",
                  label: "takes initiative",
                },
                {
                  value: "Integrated",
                  label: "works with your tools",
                },
              ].map((stat) => (
                <div
                  key={stat.value}
                  className="flex flex-col items-center text-center gap-1.5"
                >
                  <span className="font-heading text-2xl sm:text-3xl lg:text-4xl font-bold text-foreground -tracking-[0.5px]">
                    {stat.value}
                  </span>
                  <span className="text-xs sm:text-sm text-muted-foreground leading-snug max-w-32">
                    {stat.label}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </motion.section>

      {/* Agent Forger */}
      <motion.section
        initial={{ opacity: 0, y: 20 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, margin: "-100px" }}
        transition={{ duration: 0.6, ease: "easeOut" }}
        className="w-full px-6 sm:px-8 lg:px-10"
      >
        <div className="w-full max-w-424 mx-auto relative">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 70%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 30%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
            {/* Section header */}
            <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4 mt-28">
              <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                The Agent Forger
              </p>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
                Meet your AI employee
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                Define behavior, connect tools, and deploy.
                <br className="hidden sm:block" />
                Have your AI employee working alongside you today.
              </p>
            </div>

            {/* Three creation modes */}
            <div className="w-full max-w-4xl mx-auto grid grid-cols-1 sm:grid-cols-3 gap-4 px-4 lg:px-0">
              {[
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Create from scratch",
                  description:
                    "Write your own system prompt, pick a model, connect integrations, and deploy.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 00-2.455 2.456zM16.894 20.567L16.5 21.75l-.394-1.183a2.25 2.25 0 00-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 001.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 001.423 1.423l1.183.394-1.183.394a2.25 2.25 0 00-1.423 1.423z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Forge with AI",
                  description:
                    "Describe what you need. AI generates an optimized agent with the right prompt and config.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M13.5 21v-7.5a.75.75 0 01.75-.75h3a.75.75 0 01.75.75V21m-4.5 0H2.36m11.14 0H18m0 0h3.64m-1.39 0V9.349m-16.5 11.65V9.35m0 0a3.001 3.001 0 003.75-.615A2.993 2.993 0 009.75 9.75c.896 0 1.7-.393 2.25-1.016a2.993 2.993 0 002.25 1.016c.896 0 1.7-.393 2.25-1.016A3.001 3.001 0 0021 9.349m-18 0a2.999 2.999 0 00.97-1.599L5.49 3h13.02l1.52 4.75A2.999 2.999 0 0021 9.349" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Deploy instantly",
                  description:
                    "One click to deploy your agent. Connect your keys and run immediately.",
                },
              ].map((mode) => (
                <div
                  key={mode.title}
                  className="flex flex-col gap-4 rounded-2xl border border-border bg-background p-6 transition-colors"
                >
                  <div className="flex items-center justify-center w-10 h-10 rounded-xl bg-muted text-foreground">
                    {mode.icon}
                  </div>
                  <h3 className="font-heading text-base font-semibold text-foreground">
                    {mode.title}
                  </h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {mode.description}
                  </p>
                </div>
              ))}
            </div>

            {/* Steps preview */}
            <div className="w-full max-w-5xl mx-auto rounded-4xl border border-border ring-1 ring-foreground/5 shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] overflow-hidden bg-background">
              <div className="p-6 sm:p-8 lg:p-10">
                <div className="flex items-center gap-2 mb-8">
                  <span className="w-2 h-2 rounded-full bg-primary" />
                  <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-foreground">
                    How it works
                  </span>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6 lg:gap-8">
                  {[
                    {
                      step: "01",
                      title: "Pick your model",
                      description:
                        "Bring your own API key. Choose from GPT, Claude, Gemini, Llama, or any provider.",
                    },
                    {
                      step: "02",
                      title: "Add skills & tools",
                      description:
                        "Connect GitHub, Slack, Linear, and more. Select exactly which actions the agent can take.",
                    },
                    {
                      step: "03",
                      title: "Write the prompt",
                      description:
                        "Define behavior with a system prompt and instructions — or let AI generate them.",
                    },
                    {
                      step: "04",
                      title: "Deploy & run",
                      description:
                        "Your agent goes live in a sandboxed environment with full observability from day one.",
                    },
                  ].map((item) => (
                    <div key={item.step} className="flex flex-col gap-3">
                      <span className="font-mono text-xs text-primary font-medium">
                        {item.step}
                      </span>
                      <h3 className="font-heading text-base font-semibold text-foreground">
                        {item.title}
                      </h3>
                      <p className="text-sm text-muted-foreground leading-relaxed">
                        {item.description}
                      </p>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      </motion.section>

      {/* What makes an AI employee effective */}
      <motion.section
        initial={{ opacity: 0, y: 20 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, margin: "-100px" }}
        transition={{ duration: 0.6, ease: "easeOut" }}
        className="w-full px-6 sm:px-8 lg:px-10"
      >
        <div className="w-full max-w-424 mx-auto relative">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 70%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
            {/* Section header */}
            <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
              <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                AI Employee Capabilities
              </p>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
                Everything your AI employee needs to work autonomously
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                Each AI employee learns, remembers, and takes initiative — just like a human teammate.
              </p>
            </div>

            {/* Bento grid */}
            <div className="w-full max-w-5xl mx-auto grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 px-4 lg:px-0">
              {[
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Long-term memory",
                  description:
                    "Agents remember context across conversations. Persistent memory that grows smarter over time.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M14.25 6.087c0-.355.186-.676.401-.959.221-.29.349-.634.349-1.003 0-1.036-1.007-1.875-2.25-1.875s-2.25.84-2.25 1.875c0 .369.128.713.349 1.003.215.283.401.604.401.959v0a.64.64 0 01-.657.643 48.39 48.39 0 01-4.163-.3c.186 1.613.293 3.25.315 4.907a.656.656 0 01-.658.663v0c-.355 0-.676-.186-.959-.401a1.647 1.647 0 00-1.003-.349c-1.036 0-1.875 1.007-1.875 2.25s.84 2.25 1.875 2.25c.369 0 .713-.128 1.003-.349.283-.215.604-.401.959-.401v0c.31 0 .555.26.532.57a48.039 48.039 0 01-.642 5.056c1.518.19 3.058.309 4.616.354a.64.64 0 00.657-.643v0c0-.355-.186-.676-.401-.959a1.647 1.647 0 01-.349-1.003c0-1.035 1.008-1.875 2.25-1.875 1.243 0 2.25.84 2.25 1.875 0 .369-.128.713-.349 1.003-.215.283-.4.604-.4.959v0c0 .333.277.599.61.58a48.1 48.1 0 005.427-.63 48.05 48.05 0 00.582-4.717.532.532 0 00-.533-.57v0c-.355 0-.676.186-.959.401-.29.221-.634.349-1.003.349-1.035 0-1.875-1.007-1.875-2.25s.84-2.25 1.875-2.25c.37 0 .713.128 1.003.349.283.215.604.401.96.401v0a.656.656 0 00.657-.663 48.422 48.422 0 00-.37-5.36c-1.886.342-3.81.574-5.766.689a.578.578 0 01-.61-.58v0z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Skills & tools",
                  description:
                    "Plug in reusable capabilities — API calls, workflows, code execution. Compose agents from building blocks.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3l1 3m0 0l.5 1.5m-.5-1.5h-9.5m0 0l-.5 1.5m.75-9l3-3 2.148 2.148A12.061 12.061 0 0116.5 7.605" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Observability",
                  description:
                    "Traces, logs, and cost tracking for every run. Know exactly what your agents are doing and what they cost.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Access control",
                  description:
                    "Fine-grained permissions for every agent. Scope API keys, assign team roles, and control what each agent can access.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.814a4.5 4.5 0 00-1.242-7.244l4.5-4.5a4.5 4.5 0 116.364 6.364l-1.757 1.757" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Connections",
                  description:
                    "Native integrations with GitHub, Slack, Linear, Notion, and more. Select exactly which actions each agent can perform.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 013 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Sandboxed execution",
                  description:
                    "Every agent runs in an isolated environment. Shared for API-only or dedicated for full system access.",
                },
              ].map((feature) => (
                <div
                  key={feature.title}
                  className="flex flex-col gap-4 rounded-2xl border border-border bg-background p-6 transition-colors"
                >
                  <div className="flex items-center justify-center w-10 h-10 rounded-xl bg-muted text-foreground">
                    {feature.icon}
                  </div>
                  <h3 className="font-heading text-base font-semibold text-foreground">
                    {feature.title}
                  </h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {feature.description}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </motion.section>

      {/* Start with 1,000 free credits */}
      <motion.section
        initial={{ opacity: 0, y: 20 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, margin: "-100px" }}
        transition={{ duration: 0.6, ease: "easeOut" }}
        className="w-full px-6 sm:px-8 lg:px-10"
      >
        <div className="w-full max-w-424 mx-auto relative pb-20 sm:pb-28 lg:pb-36">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 70%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(ellipse 50% 60% at 50% 40%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-6 sm:gap-7 pt-20 sm:pt-28 text-center">
            <HugeiconsIcon
              icon={CpuIcon}
              size={22}
              className="text-primary"
            />
            <p className="font-mono text-[11px] font-medium uppercase tracking-[2px] text-primary">
              AI Employees Platform
            </p>
            <h2 className="font-heading text-[36px] sm:text-[52px] lg:text-[64px] font-bold text-foreground leading-[1.03] -tracking-[1px] sm:-tracking-[1.4px] max-w-3xl">
              Hire your first
              <span className="italic font-medium text-primary">
                {" "}AI employee today.
              </span>
            </h2>
            <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-xl">
              Your AI employee learns your ways of working.
            </p>
            <div className="flex flex-col items-center gap-3">
              <Link href="/demo">
                <Button size="lg" className="group cursor-pointer">
                  Book a Demo
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    size={15}
                    className="ml-1.5 opacity-80 group-hover:translate-x-0.5 transition-transform"
                  />
                </Button>
              </Link>
              <p className="font-mono text-[10px] uppercase tracking-[1.8px] text-muted-foreground/70">
                No card required · First employee free
              </p>
            </div>
          </div>
        </div>
      </motion.section>

    </div>
  )
}
