export function AgentForgerSection() {
  return (
    <section className="w-full px-4 sm:px-6 lg:px-0">
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
          <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
              The Agent Forger
            </p>
            <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
              From zero to a running agent{" "}
              <br className="hidden sm:block" />
              in under five minutes.
            </h2>
            <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
              Build from scratch, let AI forge one for you, or install from
              the marketplace. Three paths, same result — a production-ready
              agent.
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
                title: "Install from marketplace",
                description:
                  "Browse community agents. One click to install, connect your keys, and run.",
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
    </section>
  )
}
