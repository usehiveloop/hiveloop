import { AgentForgerSection } from "./_components/agent-forger-section"
import { FinalCtaSection } from "./_components/final-cta-section"
import { HeroSection } from "./_components/hero-section"
import { MarketplaceSection } from "./_components/marketplace-section"
import { PlatformFeaturesSection } from "./_components/platform-features-section"
import { PricingSection } from "./_components/pricing-section"
import { ValuePropositionSection } from "./_components/value-proposition-section"

export default function Home() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      <HeroSection />
      <ValuePropositionSection />
      <MarketplaceSection />
      <AgentForgerSection />
      <PlatformFeaturesSection />
      <PricingSection />
      <FinalCtaSection />
    </div>
  )
}
