export default function PrivacyPage() {
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
          <span className="font-mono text-mini font-medium uppercase tracking-medium text-primary">
            Legal
          </span>
          <h1 className="font-heading text-[32px] sm:text-[44px] lg:text-[56px] font-bold text-foreground leading-[1.1] -tracking-small">
            Privacy Policy
          </h1>
          <p className="text-lg sm:text-xl text-muted-foreground leading-relaxed max-w-2xl">
            Last updated: April 10, 2026
          </p>
        </div>
      </div>

      <article className="max-w-2xl mx-auto px-4 pb-24">
        <div className="prose-custom flex flex-col gap-6">
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            At HiveLoop, we take your privacy seriously. This policy explains how we collect, use, and protect your information.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            1. Information We Collect
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We collect information you provide directly: account details (email, name), payment information, and any content you submit through the platform.
          </p>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We also collect usage data including API calls, agent execution logs, and device information to improve our service.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            2. How We Use Your Information
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We use your information to provide and improve the service, process payments, communicate about your account, and ensure platform security.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            3. Data Retention
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We retain your data as long as your account is active. Upon deletion, we delete or anonymize your data within 30 days, except where retention is required by law.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            4. Data Security
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We use encryption (AES-256-GCM) for credential storage, TLS for data in transit, and access controls to protect your data. No third party ever has access to your API keys.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            5. Cookies
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We use essential cookies for authentication and session management. Analytics cookies are optional and help us understand how users interact with the platform.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            6. Third-Party Services
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We use third-party services for payment processing (Stripe), infrastructure (Daytona), and analytics. Each has their own privacy policies governing their use of your data.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            7. Your Rights
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            You have the right to access, correct, or delete your data. You can export your data at any time through your account settings. Contact privacy@hiveloop.com for requests.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            8. Changes to Policy
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            We may update this policy periodically. We will notify you of significant changes via email or platform notification.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-micro">
            Contact Us
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            For privacy-related questions, contact our Data Protection Officer at privacy@hiveloop.com.
          </p>
        </div>
      </article>
    </div>
  )
}
