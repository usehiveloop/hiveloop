import { Nav } from "@/components/nav";
import { Footer } from "@/components/footer";
import { PricingContent } from "@/components/pricing-content";

function PricingHero() {
  return (
    <section className="flex flex-col items-center gap-5 px-20 pt-25 pb-20">
      <span className="font-mono text-[13px] font-medium leading-4 tracking-widest uppercase text-primary">
        Pricing
      </span>
      <h1 className="max-w-175 text-center font-mono text-5xl font-medium leading-14 text-[#E4E1EC]">
        Pay per request. Start free.
      </h1>
      <p className="max-w-140 text-center text-lg leading-7 text-[#9794A3]">
        Free up to 10,000 requests. Then simple per-request pricing that drops as you scale. No per-seat charges, no credential limits.
      </p>
    </section>
  );
}

type FAQ = { question: string; answer: string };

function FAQColumn({ questions }: { questions: FAQ[] }) {
  return (
    <div className="flex flex-1 flex-col gap-10">
      {questions.map((q, i) => (
        <div key={i}>
          <div className="flex flex-col gap-3">
            <span className="font-mono text-base font-medium leading-5 text-[#E4E1EC]">
              {q.question}
            </span>
            <span className="text-[15px] leading-6 text-[#9794A3]">
              {q.answer}
            </span>
          </div>
          {i < questions.length - 1 && <div className="mt-10 h-px w-full bg-border" />}
        </div>
      ))}
    </div>
  );
}

function FAQSection() {
  const leftQuestions: FAQ[] = [
    {
      question: "What counts as a proxy request?",
      answer: "Every request forwarded through LLMVault to an LLM provider. Credential storage, token minting, and revocations don\u2019t count.",
    },
    {
      question: "What happens if I exceed my tier?",
      answer: "You\u2019ll get an email at 80% usage. Overages are billed at your tier\u2019s per-request rate. No service interruption, no surprise bills.",
    },
    {
      question: "Can I change tiers anytime?",
      answer: "Yes. Upgrade or downgrade at any time. Upgrades are prorated. Downgrades take effect at the next billing cycle.",
    },
  ];

  const rightQuestions: FAQ[] = [
    {
      question: "Is the free tier really free?",
      answer: "Yes. No credit card, no trial expiration. 10,000 requests/month, forever. Build your integration and upgrade when you have real traffic.",
    },
    {
      question: "Are credentials limited on paid tiers?",
      answer: "No. All paid tiers include unlimited credentials and unlimited token mints. You only pay for proxy requests.",
    },
    {
      question: "What does self-hosted include?",
      answer: "Docker image, Helm chart, deployment guide, and dedicated support. You bring your own Vault, Postgres, and Redis.",
    },
  ];

  return (
    <section className="relative flex flex-col gap-16 overflow-clip bg-surface px-20 py-25">
      <div className="absolute top-0 left-0 h-px w-full bg-border" />

      <div className="flex flex-col items-center gap-4">
        <span className="font-mono text-[13px] font-medium leading-4 tracking-widest uppercase text-primary">
          FAQ
        </span>
        <h2 className="text-center font-mono text-4xl font-medium leading-11 text-[#E4E1EC]">
          Common questions.
        </h2>
      </div>

      <div className="flex gap-10">
        <FAQColumn questions={leftQuestions} />
        <FAQColumn questions={rightQuestions} />
      </div>
    </section>
  );
}

export default function PricingPage() {
  return (
    <div className="mx-auto flex min-h-screen max-w-360 flex-col bg-background">
      <Nav />
      <PricingHero />
      <PricingContent />
      <FAQSection />
      <Footer />
    </div>
  );
}
