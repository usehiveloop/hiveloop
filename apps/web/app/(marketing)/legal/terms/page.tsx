export default function TermsPage() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      <div className="relative w-full overflow-hidden">
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background: "radial-gradient(ellipse at 50% 80%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 60%)",
          }}
        />
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            backgroundImage:
              "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
            backgroundSize: "60px 60px",
            maskImage: "radial-gradient(ellipse at 50% 100%, black 10%, transparent 60%)",
          }}
        />
        <div className="relative max-w-3xl mx-auto px-4 pt-16 sm:pt-24 pb-16 sm:pb-20 flex flex-col items-center text-center gap-6">
          <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
            Legal
          </span>
          <h1 className="font-heading text-[32px] sm:text-[44px] lg:text-[56px] font-bold text-foreground leading-[1.1] -tracking-[1px]">
            Terms of Service
          </h1>
          <p className="text-lg sm:text-xl text-muted-foreground leading-relaxed max-w-2xl">
            Last updated: April 10, 2026
          </p>
        </div>
      </div>

      <article className="max-w-2xl mx-auto px-4 pb-24">
        <div className="prose-custom flex flex-col gap-6">
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            Welcome to ZiraLoop. By accessing or using our platform, you agree to be bound by these Terms of Service.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            1. Acceptance of Terms
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            By creating an account or using ZiraLoop, you agree to these terms and our Privacy Policy. If you do not agree, do not use our services.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            2. Use of Service
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            You may use ZiraLoop to create and deploy AI agents. You are responsible for the content and actions of agents you create. You agree not to use the platform for illegal purposes or to violate any third-party rights.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            3. Account Responsibilities
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            You are responsible for maintaining the confidentiality of your account credentials and for all activities under your account. Notify us immediately of any unauthorized use.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            4. Fees and Payment
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            Subscription fees are billed in advance. All fees are non-refundable except as required by law. We reserve the right to change pricing with 30 days notice.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            5. Intellectual Property
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            You retain ownership of content you create. We claim no ownership over your agents or data. However, you grant us a license to operate your agents as part of the service.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            6. Limitation of Liability
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            ZiraLoop is provided &ldquo;as is&rdquo; without warranties. We are not liable for any indirect, incidental, or consequential damages arising from your use of the platform.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            7. Termination
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We may terminate or suspend your account at any time for violation of these terms. You may cancel your subscription at any time through your account settings.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            8. Changes to Terms
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We may update these terms from time to time. Continued use after changes constitutes acceptance of the new terms.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            Contact Us
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            For questions about these terms, contact us at legal@ziraloop.com.
          </p>
        </div>
      </article>
    </div>
  )
}
