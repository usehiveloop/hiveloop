import type { Metadata } from "next"
import { LegalPage } from "../_components/legal-page"

export const metadata: Metadata = {
  title: "Terms of Service",
  description:
    "Terms governing use of Hivy, the AI employee platform operated by HIVY TECHNOLOGIES LTD.",
}

const relatedLinks = [
  { label: "Legal", href: "/legal" },
  { label: "Privacy", href: "/legal#privacy-policy" },
  { label: "DPA", href: "/legal#data-processing-addendum" },
  { label: "Acceptable Use", href: "#acceptable-use" },
  { label: "AI", href: "#ai-responsibilities" },
]

const sections = [
  {
    id: "agreement",
    title: "Agreement",
    body: [
      "These Terms of Service govern access to and use of Hivy, including our website, web application, APIs, AI employee platform, integrations, knowledge base, sandboxes, webhooks, billing tools, support, and related services.",
      "Hivy is operated by HIVY TECHNOLOGIES LTD, a company incorporated in Nigeria. Registered number: 1078455345. Registered office: B1 Orji Murray St, behind Romay Garden Estate, off Lekki - Epe Expressway, Eti-Osa, Lekki 106104, Lagos.",
      "By creating an account, joining a workspace, connecting a third party service, paying for a plan, or otherwise using Hivy, you agree to these Terms on behalf of yourself and, where applicable, the organisation you represent.",
      "If you do not agree, you must not use Hivy. If you use Hivy for an organisation, you confirm that you have authority to bind that organisation.",
      "Last updated: May 28, 2026.",
    ],
  },
  {
    id: "business-use",
    title: "Business use",
    body: [
      "Hivy is built for business and professional use. You must be at least 18 years old to use Hivy.",
      "Individuals may use Hivy for business, professional, or work-related purposes, including as solo founders, freelancers, contractors, or representatives of an organisation.",
      "Nothing in these Terms limits mandatory consumer rights that cannot lawfully be waived.",
    ],
  },
  {
    id: "service",
    title: "The Hivy service",
    body: [
      "Hivy helps teams create and operate AI employees. AI employees can receive instructions, use approved tools, read connected knowledge, interact with integrated services, run sandboxed workflows, browse or research the web where enabled, create files, trigger actions, and produce outputs for review.",
      "Hivy is not a substitute for professional judgement, legal advice, medical advice, financial advice, cybersecurity approval, compliance sign-off, regulated employment decisions, or any decision that must legally or practically be made by a qualified human.",
      "Some features may be experimental, beta, limited availability, or dependent on third party providers. We may impose usage limits, rate limits, storage limits, safety controls, or approval requirements where needed for security, reliability, compliance, or fair use.",
    ],
  },
  {
    id: "accounts",
    title: "Accounts and workspaces",
    body: [
      "You must provide accurate account information and keep your login credentials secure.",
      "You are responsible for activity under your account, your API keys, your workspace, your connected services, and users you invite.",
      "Workspace administrators are responsible for assigning roles, removing users who should no longer have access, reviewing integrations, managing scopes, and ensuring workspace use complies with law and internal policies.",
    ],
  },
  {
    id: "customer-data",
    title: "Customer Data",
    body: [
      "Customer Data means prompts, instructions, messages, files, knowledge sources, credentials, API keys, integration data, outputs, workflow payloads, sandbox files, and other content submitted to or processed through Hivy by or for your workspace.",
      "You retain ownership of Customer Data. You grant Hivy the limited rights needed to host, secure, process, transmit, transform, index, retrieve, display, generate outputs from, and otherwise operate Customer Data to provide Hivy, support users, prevent abuse, comply with law, and enforce these Terms.",
      "You are responsible for having all rights, notices, consents, lawful bases, contracts, and authority needed to submit Customer Data to Hivy and to allow Hivy, AI providers, subprocessors, integrations, and sandboxes to process it.",
    ],
  },
  {
    id: "ai-responsibilities",
    title: "AI responsibilities",
    body: [
      "AI outputs may be inaccurate, incomplete, outdated, offensive, unsafe, biased, infringing, or unsuitable for your intended use.",
      "You are responsible for reviewing outputs before relying on them, publishing them, sending them to customers, committing code, changing infrastructure, making business decisions, or taking actions that affect people, property, systems, accounts, or legal rights.",
      "You must not use Hivy as the sole basis for decisions with legal, employment, credit, education, healthcare, housing, insurance, law enforcement, immigration, public-service access, or similarly significant effects on a person.",
      "Where an AI employee can take actions in connected tools, you are responsible for configuring permissions carefully, limiting scopes, requiring approvals for sensitive actions, monitoring activity, and revoking access when no longer needed.",
    ],
  },
  {
    id: "approvals-autonomy",
    title: "Approvals and autonomous actions",
    body: [
      "Hivy can operate with different levels of autonomy depending on your workspace configuration, connected services, workflow settings, approval rules, and AI employee permissions.",
      "Hivy may perform read-only operations, retrieve context, draft work, generate reports, create files, and prepare suggested actions without requiring per-action approval.",
      "Actions that modify external systems, send messages, delete or update records, spend money, change permissions, deploy code, affect infrastructure, or have significant business impact should be configured to require approval from authorised users.",
      "If you enable auto-approval, recurring workflows, shared integration permissions, schedules, or similar controls, you authorise Hivy to execute covered actions without per-action human approval.",
      "You are responsible for ensuring that only authorised users can approve actions or configure pre-authorised workflows. Once an action is approved or allowed by your configuration, you are responsible for the consequences of that action.",
      "You are responsible for promptly updating or revoking approvals, integration connections, permissions, and workflow settings if personnel, roles, or business requirements change.",
    ],
  },
  {
    id: "acceptable-use",
    title: "Acceptable use",
    body: ["You must use Hivy lawfully and responsibly. You must not use Hivy to:"],
    list: [
      "violate Nigerian law or any law that applies to you;",
      "gain unauthorised access to systems, accounts, networks, devices, data, payment instruments, credentials, or third party services;",
      "create, deploy, distribute, test, or assist malware, ransomware, credential theft, phishing, spam, fraud, scams, impersonation, unlawful scraping, or deceptive automation;",
      "intercept communications unlawfully or collect traffic data, subscriber information, content data, tokens, or secrets without authority;",
      "submit personal data without a lawful basis, required notices, and authority;",
      "submit sensitive personal data, children's data, regulated data, production secrets, or sensitive operational access unless you have implemented appropriate safeguards and have authority to do so;",
      "generate or distribute unlawful, defamatory, discriminatory, sexually exploitative, child safety, violent, terrorist, hateful, deceptive, or rights-infringing content;",
      "bypass rate limits, security controls, payment controls, sandbox restrictions, approval flows, or monitoring systems;",
      "test Hivy for vulnerabilities except through a written programme or prior written permission from Hivy;",
      "resell, sublicense, white-label, or provide Hivy as a managed service unless we agree in writing.",
    ],
  },
  {
    id: "integrations",
    title: "Integrations and third parties",
    body: [
      "Hivy may connect to third party services through OAuth, API keys, webhooks, or integration providers. Your use of those services remains governed by their own terms and policies.",
      "When you connect a third party service, you authorise Hivy to access and process the data and permissions exposed through that connection for your workspace.",
      "Integrations connected in a workspace may be workspace-shared by design. Authorised workspace members may use those integrations and related tools through Hivy, and actions may execute using the connected account's permissions.",
      "If you connect an integration, you represent that you are authorised by your organisation and, where applicable, the relevant third party account owner to grant that access. You are responsible for granted scopes, account-level permissions, and resulting access to data and actions available to your workspace through Hivy.",
      "Third party services may change, suspend, rate limit, reject, or revoke access at any time. Hivy is not responsible for third party service outages, policy changes, data quality, fees, security incidents, or actions outside Hivy's control.",
    ],
  },
  {
    id: "sandboxes",
    title: "Sandboxes and generated work",
    body: [
      "Some AI employees run in sandboxed runtimes that can execute commands, browse websites, inspect repositories, operate files, call APIs, and use configured tools. Sandboxing reduces risk, but it does not eliminate risk.",
      "You must not place production secrets, regulated data, or sensitive operational access in a sandbox unless you have assessed the risk, configured appropriate controls, and have authority to do so.",
      "You are responsible for reviewing generated code, infrastructure changes, messages, documents, automations, pull requests, scripts, and other work product before use or deployment.",
    ],
  },
  {
    id: "billing",
    title: "Billing",
    body: [
      "Paid plans, usage credits, add-ons, or other charges are presented at checkout, in the product, or in an order form. Unless stated otherwise, subscriptions renew monthly and charges are processed through Paystack.",
      "You authorise Hivy and its payment provider to charge your selected payment method for recurring fees, usage, upgrades, taxes, and other amounts due.",
      "If payment fails, we may retry the charge, restrict paid features, suspend the workspace, or cancel the subscription.",
      "We may change pricing, introduce new plans, add premium features, or move features between plans. If a material pricing change affects your current paid plan, we will provide at least 30 days' advance notice by email, product notice, or another reasonable channel.",
      "Cancellation takes effect at the end of the current billing period unless the product, checkout, or order form says otherwise. Refunds are provided only where required by law, stated in a separate agreement, or approved by Hivy in writing.",
      "Hivy does not offer refunds at this time except where required by law or expressly approved by Hivy in writing. Hivy does not offer trial terms at this time. Credits expire at the end of the applicable subscription period. Taxes and VAT are handled according to the checkout, invoice, or applicable law.",
    ],
  },
  {
    id: "intellectual-property",
    title: "Intellectual property",
    body: [
      "Hivy and its licensors own the service, software, designs, documentation, prompts supplied by Hivy, system skills, integrations, trademarks, logos, and other platform materials. These Terms do not transfer Hivy intellectual property to you.",
      "Subject to your compliance with these Terms and payment of applicable fees, Hivy grants you a limited, revocable, non-exclusive, non-transferable right to use the service for your internal business purposes.",
      "You own Customer Data. As between you and Hivy, you own outputs generated specifically for your workspace to the extent ownership can lawfully vest in you, subject to third party rights, open source licences, model provider terms, and applicable law.",
      "Hivy does not guarantee that AI outputs are copyrightable, non-infringing, exclusive, accurate, or free from third party rights.",
      "If you send feedback, suggestions, feature requests, or improvement ideas, Hivy may use them without restriction or compensation.",
    ],
  },
  {
    id: "privacy-security",
    title: "Privacy and security",
    body: [
      "The Legal page explains how Hivy handles privacy, data processing, security, cookies, and subprocessors.",
      "We use technical and organisational measures designed to protect Hivy. No online service is perfectly secure.",
      "You must promptly notify security@usehivy.com if you believe your account, workspace, credentials, API keys, tokens, sandbox, or integration has been compromised.",
    ],
  },
  {
    id: "confidentiality",
    title: "Confidentiality",
    body: [
      "Each party may receive non-public business, technical, security, financial, product, or operational information from the other party.",
      "The receiving party must use confidential information only for the purpose of using or providing Hivy, protect it with reasonable care, and disclose it only to people and providers who need to know and are bound by appropriate confidentiality duties.",
      "Confidentiality duties do not apply to information that is public without breach, already known without restriction, independently developed, or lawfully received from a third party.",
    ],
  },
  {
    id: "availability",
    title: "Availability and changes",
    body: [
      "We aim to provide a reliable service, but we do not guarantee uninterrupted access, error-free operation, exact output quality, perpetual availability of integrations, or that every AI action will complete successfully.",
      "Hivy may change, improve, suspend, or discontinue features. We may also impose or change usage limits, rate limits, storage limits, safety controls, or approval requirements.",
    ],
  },
  {
    id: "suspension",
    title: "Suspension and termination",
    body: [
      "You may stop using Hivy or cancel a paid plan through the product where available.",
      "We may suspend or terminate access immediately if we believe you have breached these Terms, created security or legal risk, failed to pay, used the service abusively, or caused harm to Hivy, users, third party services, or the public.",
      "After termination, your right to use Hivy ends. We may retain or delete data according to the Legal page, legal obligations, backup cycles, security needs, and any written agreement.",
    ],
  },
  {
    id: "disclaimers",
    title: "Disclaimers",
    body: [
      "To the maximum extent allowed by law, Hivy is provided on an as available basis without warranties of merchantability, fitness for a particular purpose, non-infringement, uninterrupted operation, error-free operation, exact availability of third party services, or that AI outputs will be accurate, safe, lawful, original, or suitable.",
      "Nothing in these Terms excludes any warranty, condition, or liability that cannot lawfully be excluded under applicable law.",
    ],
  },
  {
    id: "liability",
    title: "Liability",
    body: [
      "To the maximum extent allowed by law, Hivy will not be liable for indirect, incidental, special, consequential, exemplary, punitive, or lost profit damages, loss of goodwill, loss of data, business interruption, third party claims, or costs of substitute services arising from or related to Hivy.",
      "To the maximum extent allowed by law, Hivy's aggregate liability for all claims relating to the service will not exceed the amounts you paid to Hivy for the service in the three months before the event giving rise to the claim, or NGN 100,000 if you used only free services.",
      "Nothing in these Terms limits liability that cannot lawfully be limited.",
    ],
  },
  {
    id: "indemnity",
    title: "Indemnity",
    body: [
      "You will indemnify and hold harmless HIVY TECHNOLOGIES LTD, its directors, officers, employees, contractors, and affiliates from claims, losses, liabilities, damages, penalties, costs, and expenses arising from your Customer Data, your instructions to AI employees, your connected services, your sandbox activity, your breach of these Terms, your violation of law, or your infringement of third party rights.",
    ],
  },
  {
    id: "law",
    title: "Governing law and disputes",
    body: [
      "These Terms are governed by the laws of the Federal Republic of Nigeria.",
      "The parties will first try to resolve disputes in good faith by written notice and discussion for at least 30 days.",
      "If a dispute is not resolved through discussion, the Nigerian legal system will apply and the courts located in Lagos State, Nigeria will have jurisdiction, subject to any mandatory consumer, data protection, regulatory, cybercrime, or injunctive rights that cannot lawfully be limited.",
    ],
  },
  {
    id: "general-terms",
    title: "General terms",
    body: [
      "These Terms, together with the Legal page and any applicable order form, are the entire agreement between you and Hivy for the service.",
      "If any provision of these Terms is held invalid or unenforceable, the remaining provisions will remain in effect.",
      "Our failure to enforce any right or provision of these Terms is not a waiver of that right or provision.",
      "You may not assign these Terms without our prior written consent. We may assign these Terms as part of a merger, acquisition, financing, restructuring, sale of assets, corporate reorganisation, or by operation of law.",
      "No joint venture, partnership, employment, fiduciary, or agency relationship exists between you and Hivy because of these Terms or your use of the service.",
    ],
  },
  {
    id: "contact",
    title: "Contact",
    body: [
      "HIVY TECHNOLOGIES LTD, registered number 1078455345.",
      "Registered office: B1 Orji Murray St, behind Romay Garden Estate, off Lekki - Epe Expressway, Eti-Osa, Lekki 106104, Lagos.",
      "Legal questions: legal@usehivy.com. Privacy requests: hello@usehivy.com. Security reports: security@usehivy.com.",
    ],
  },
  {
    id: "changes-contact",
    title: "Changes and contact",
    body: [
      "We may update these Terms from time to time. If changes are material, we will take reasonable steps to notify users through the product, email, website, or another appropriate channel.",
      "Continued use after the effective date of an update means you accept the updated Terms, except where law requires additional consent or notice.",
      "For legal questions, contact legal@usehivy.com. For privacy requests, contact hello@usehivy.com. For security reports, contact security@usehivy.com.",
    ],
  },
]

export default function TermsPage() {
  return (
    <LegalPage
      title="Terms of Service"
      eyebrow="Terms"
      effectiveDate="May 28, 2026"
      lastUpdated="May 28, 2026"
      version="1"
      notice="These Terms govern use of Hivy. The Legal page contains our Privacy Policy, Data Processing Addendum, Security notice, Cookie notice, and Subprocessor List."
      relatedLinks={relatedLinks}
      sections={sections}
    />
  )
}
