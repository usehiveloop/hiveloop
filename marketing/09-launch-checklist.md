# ZiraLoop — Launch Checklist

---

## Pre-Launch (4-6 Weeks Before)

### Product
- [ ] Core proxy working end-to-end (store → mint → proxy → revoke)
- [ ] Cloud-hosted service deployed and stable
- [ ] Free tier provisioning automated (signup → org created → API key returned)
- [ ] Quickstart path tested: signup → first proxy request in < 15 minutes
- [ ] Dashboard MVP (view credentials, tokens, basic usage stats)
- [ ] Error messages are clear and actionable

### Website
- [ ] Homepage live with copy from `05-homepage-copy.md`
- [ ] Pricing page live
- [ ] Security page live
- [ ] Documentation site live:
  - [ ] Quickstart guide
  - [ ] Architecture overview
  - [ ] API reference (all endpoints)
  - [ ] Provider-specific guides (OpenAI, Anthropic, Google)
  - [ ] Security model documentation
- [ ] Blog ready with 5 launch posts (see `06-content-strategy.md`)
- [ ] SEO fundamentals: meta tags, OG tags, sitemap, robots.txt
- [ ] Analytics installed (Plausible or PostHog)
- [ ] Conversion tracking: signup events, quickstart completion

### Content
- [ ] Launch blog post: "Introducing ZiraLoop" — drafted and reviewed
- [ ] HN "Show HN" post drafted
- [ ] Twitter/X launch thread drafted
- [ ] Reddit posts drafted (r/programming, r/devops, r/SideProject)
- [ ] 4 supporting blog posts published before launch day
- [ ] Dev newsletter submissions sent (TLDR, Bytes, Changelog) 1-2 weeks early

### Community
- [ ] Beta testers have used the product for 2+ weeks
- [ ] 2-3 testimonial quotes collected from beta users
- [ ] Discord or Slack community created (optional for launch, useful for post-launch)

### Operations
- [ ] Support email set up (support@ziraloop.com)
- [ ] Monitoring and alerting in place (uptime, error rates, latency)
- [ ] Status page live (e.g., Instatus, Statuspage)
- [ ] Legal: Terms of Service, Privacy Policy published
- [ ] Billing integration working (Stripe)

---

## Launch Day

### Morning (Before HN Post)
- [ ] Publish launch blog post
- [ ] Publish Twitter/X launch thread
- [ ] Post to Reddit
- [ ] Submit to Product Hunt (if doing same-day)
- [ ] Email waitlist: "ZiraLoop is live"
- [ ] Cross-post launch blog to Dev.to and Hashnode

### HN Post (Target: Tuesday 9am ET)
- [ ] Post "Show HN: ZiraLoop — Secure proxy layer for LLM API keys"
- [ ] Founder in HN comments for 6+ hours answering questions
- [ ] Prepared answers for likely questions:
  - "Why not just use Vault directly?"
  - "What's the latency overhead?"
  - "Is this open source?"
  - "How does this compare to LiteLLM?"
  - "What about vendor lock-in?"
  - "How are you handling key rotation?"

### Monitoring
- [ ] Watch error rates and latency during traffic spike
- [ ] Monitor signup flow for issues
- [ ] Check quickstart path works under load
- [ ] Respond to support emails within 2 hours

---

## Post-Launch (Week 1-2)

### Analyze
- [ ] Review signup numbers and conversion funnel
- [ ] Identify where users drop off (signup → quickstart → first proxy)
- [ ] Read all HN/Reddit/Twitter comments for product feedback
- [ ] Survey new signups: "What made you try ZiraLoop?"

### Iterate
- [ ] Fix any issues discovered during launch
- [ ] Improve quickstart based on where users got stuck
- [ ] Update docs based on common questions
- [ ] Write a "What We Learned from Launch" blog post (optional, good for transparency)

### Sustain
- [ ] Resume weekly blog posting cadence
- [ ] Begin outreach to AI dev tool companies for partnerships
- [ ] Start tracking activation metrics (time-to-first-proxy-request)
- [ ] Plan first case study with an active early user

---

## Success Criteria (First 30 Days)

| Metric | Target |
|--------|--------|
| Website unique visitors | 10,000+ |
| Free tier signups | 500+ |
| Users who complete quickstart | 200+ |
| Users who make first proxy request | 100+ |
| HN front page | Yes |
| Blog post views (total) | 20,000+ |
| Twitter/X impressions | 100,000+ |
| Support tickets | < 50 (low = good docs) |
| Critical bugs | 0 |
| Uptime | 99.9%+ |
