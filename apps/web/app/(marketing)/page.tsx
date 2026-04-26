import { HeroSection } from "./_components/hero-section"
import { ValuePropSection } from "./_components/value-prop-section"
import { MarketplaceSection } from "./_components/marketplace-section"
import { AgentForgerSection } from "./_components/agent-forger-section"
import { PlatformFeaturesSection } from "./_components/platform-features-section"
import { CtaSection } from "./_components/cta-section"

export default function Home() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      <HeroSection />
      <ValuePropSection />
      <MarketplaceSection />
      <AgentForgerSection />
      <PlatformFeaturesSection />
      <CtaSection />
    </div>
  )
}
