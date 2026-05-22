import type { Metadata } from "next"
import { LegalPage } from "../_components/legal-page"

export const metadata: Metadata = {
  title: "Legal",
  description:
    "Legal notices for Hivy, including privacy, data processing, security, cookies, and subprocessors.",
}

const sections = [
  {
    id: "overview",
    title: "Overview",
    body: [
      "This page explains how HIVY TECHNOLOGIES LTD handles privacy, data processing, security, cookies, and service-provider disclosures for Hivy.",
      "Hivy is operated by HIVY TECHNOLOGIES LTD, a company incorporated in Nigeria. Registered number: 1078455345. Registered office: B1 Orji Murray St, behind Romay Garden Estate, off Lekki - Epe Expressway, Eti-Osa, Lekki 106104, Lagos.",
      "For privacy requests, legal notices, security reports, and other legal questions, contact hello@usehivy.com.",
      "Last updated: May 28, 2026.",
    ],
  },
  {
    id: "privacy-policy",
    title: "Privacy Policy",
    body: [
      "Hivy helps teams create AI employees that can use approved tools, read connected knowledge, operate integrations, run sandboxed tasks, and generate work for review.",
      "We collect personal data from you, workspace administrators, connected services, payment providers, identity providers, devices, logs, and support or sales communications.",
      "We use personal data to provide Hivy, operate workspaces, authenticate users, process payments, run AI and automation features, connect third party tools, secure the service, prevent abuse, provide support, improve reliability, and comply with law.",
    ],
  },
  {
    id: "data-we-collect",
    title: "Data we collect",
    body: ["Depending on how Hivy is used, we may process:"],
    list: [
      "Account data, such as name, email address, login method, password hash, workspace membership, roles, invitations, and account status.",
      "Workspace data, such as organisation name, members, settings, AI employee configurations, prompts, instructions, skills, schedules, permissions, approvals, conversations, messages, outputs, tool calls, generated files, and usage metrics.",
      "Connected service data from tools a workspace connects, such as Google Drive, Google Sheets, GitHub, Slack, Stripe, cloud platforms, issue trackers, documentation tools, or other integrations.",
      "Knowledge data, such as uploaded files, crawled websites, indexed documents, chunks, embeddings, search queries, and retrieval results.",
      "Credential metadata and encrypted secrets, such as API keys, OAuth tokens, environment variables, labels, providers, scopes, and revocation status.",
      "Billing data, such as plan, credits, payment references, Paystack customer or transaction identifiers, invoice data, taxes, and subscription status.",
      "Technical and security data, such as IP address, device and browser information, request logs, API usage, errors, performance traces, rate-limit events, and abuse signals.",
      "Support and communication data, such as emails, messages, feedback, bug reports, attachments, and marketing preferences.",
    ],
  },
  {
    id: "legal-bases",
    title: "Legal bases",
    body: [
      "Where Nigerian data protection law applies, we process personal data under the legal basis that fits the context.",
    ],
    list: [
      "Contract: to create accounts, provide Hivy, operate workspaces, run AI employees, process billing, and provide support.",
      "Consent: where you connect optional integrations, approve optional cookies, subscribe to optional marketing, or otherwise consent to a feature.",
      "Legitimate interests: to secure Hivy, prevent abuse, improve reliability, debug errors, communicate with business users, and understand product usage.",
      "Legal obligations: to comply with tax, accounting, corporate, cybercrime, data protection, court, regulator, and other legal duties.",
    ],
  },
  {
    id: "ai-processing",
    title: "AI processing",
    body: [
      "Hivy sends prompts, files, retrieved knowledge, tool results, connected service data, and related context to AI model, embedding, inference, reranking, or runtime providers when needed to complete a user request.",
      "We do not use Customer Data to train a Hivy-owned foundation model. Third party AI providers are not permitted by Hivy to retain Customer Data or use it for training, evaluation, abuse monitoring, or service improvement beyond what is necessary to process the specific user request.",
      "AI outputs may be inaccurate, incomplete, outdated, biased, unsafe, or unsuitable. Workspace owners and users are responsible for reviewing outputs before relying on them or using them externally.",
    ],
  },
  {
    id: "google-data",
    title: "Google user data",
    body: [
      "If a workspace connects Google services, Hivy may access Google user data made available by the scopes approved by the user or administrator, such as profile information, file metadata, file content, spreadsheet data, permissions, and folder structure.",
      "Hivy uses Google user data only to provide or improve user-facing features requested by the user or workspace, such as authentication, file access, retrieval, indexing, summarisation, tool execution, and workflow automation.",
      "Hivy's use and transfer of information received from Google APIs will adhere to the Google API Services User Data Policy, including the Limited Use requirements.",
      "We do not sell Google user data, use it for advertising, or transfer it to data brokers. Google login uses the openid, email, and profile scopes. Optional Google integrations are connected only when a user or workspace administrator authorises them and may include Google Drive, Google Sheets, Gmail, Google Docs, Google Tasks, Google Cloud, Google Slides, Google Calendar, Firebase, and related Google APIs requested for the selected integration.",
    ],
  },
  {
    id: "end-user-policy",
    title: "End-user Policy",
    body: [
      "Some personal data processed by Hivy may belong to people who do not have a Hivy account. For example, a customer may add employee records, customer messages, documents, tickets, repository content, or other business data to a workspace.",
      "When Hivy processes that data for a workspace, the workspace owner is usually responsible for deciding why and how the data is processed. Hivy follows the workspace owner's instructions unless we need to process the data for security, abuse prevention, legal compliance, billing, or service integrity.",
      "If your data was submitted to Hivy by a company, employer, contractor, customer, or other workspace owner, contact that workspace owner first. You may also contact hello@usehivy.com and we will help route the request where appropriate.",
    ],
  },
  {
    id: "customer-policy",
    title: "Customer Policy",
    body: [
      "Workspace owners and administrators are responsible for the data, prompts, files, integrations, credentials, permissions, users, and AI employee instructions they add to Hivy.",
      "Customers must have the required rights, notices, consents, lawful bases, contracts, and authority before submitting personal data, confidential information, third party platform data, employee records, customer records, or connected service data to Hivy.",
      "Customers must configure integrations and AI employee permissions carefully, review outputs before use, and avoid submitting sensitive or regulated data unless they have assessed the risk and implemented appropriate safeguards.",
    ],
  },
  {
    id: "data-processing-addendum",
    title: "Data Processing Addendum",
    body: [
      "When Hivy processes Customer Data on behalf of a workspace owner, the workspace owner is the controller or processor of that data and Hivy acts as processor or subprocessor.",
      "Hivy will process Customer Data to provide, secure, support, and improve Hivy, and to follow instructions given through the product, APIs, workspace settings, support requests, and customer agreements.",
      "Hivy may use subprocessors to provide the service. We remain responsible for using service providers that support appropriate confidentiality, security, and data protection obligations.",
      "If we become aware of a personal data breach affecting Customer Data, we will notify affected customers without undue delay and provide information reasonably available to help them meet their legal obligations.",
      "On request or account closure, Hivy will delete or export eligible Customer Data according to product capabilities, legal obligations, backup cycles, security needs, and any written agreement.",
    ],
  },
  {
    id: "subprocessors",
    title: "Subprocessors",
    body: [
      "Hivy uses service providers to host, secure, monitor, bill, email, integrate, and operate the service. We will keep this list current as production vendors change.",
    ],
    list: [
      "Hetzner and Railway - hosting, infrastructure, databases, files, and backups.",
      "OpenRouter and configured model providers such as OpenAI, Anthropic, Google, and Fireworks - model inference, embeddings, reranking, or related AI processing.",
      "In-house integration/OAuth systems, including self-hosted Nango - optional integration connection and token management.",
      "Paystack - billing, checkout, payment references, and subscription data.",
      "Kibamail - transactional email and support communications.",
      "In-house monitoring/error systems - logs, errors, traces, and performance data.",
      "In-house analytics - product analytics and diagnostics.",
    ],
  },
  {
    id: "sharing",
    title: "Sharing",
    body: [
      "We share personal data only where needed to provide Hivy, follow customer instructions, comply with law, protect rights, or operate the business.",
      "Recipients may include AI providers, connected services, payment providers, hosting and infrastructure providers, monitoring and support providers, professional advisers, regulators, courts, law enforcement, authorised government agencies, and business successors in a merger, acquisition, financing, restructuring, or asset sale.",
    ],
  },
  {
    id: "international-transfers",
    title: "International transfers",
    body: [
      "Hivy may process and store personal data in Nigeria and other countries where Hivy, our providers, AI providers, infrastructure providers, support providers, or connected services operate.",
      "Where Nigerian data protection law requires safeguards for international transfers, we use appropriate safeguards such as contracts, data processing terms, transfer assessments, security controls, access restrictions, and other measures recognised by applicable law.",
      "Production data is primarily hosted or accessed in Helsinki, Finland through Hetzner and in the United States through Railway.",
    ],
  },
  {
    id: "retention",
    title: "Retention",
    body: [
      "We keep personal data for as long as needed to provide Hivy, maintain workspaces, comply with law, resolve disputes, enforce agreements, prevent fraud, keep security records, and maintain backups.",
      "Workspace data is generally retained while the workspace is active. When an account is closed or we receive a validated deletion request, we delete eligible Customer Data from active production systems within 30 days.",
      "Some data may remain for a limited period after deletion because of backups, audit logs, billing records, security records, legal holds, or technical constraints. Backups are retained only for business continuity and are removed or overwritten on their normal rotation.",
    ],
  },
  {
    id: "rights",
    title: "Your rights",
    body: [
      "Subject to applicable law and verification, you may have rights to access your personal data, receive information about processing, correct inaccurate data, request deletion, restrict processing, object to processing, withdraw consent, request portability, object to direct marketing, and complain to the Nigeria Data Protection Commission.",
      "To exercise rights, contact hello@usehivy.com. We may ask for information to verify your identity and locate the relevant account, workspace, or data.",
    ],
  },
  {
    id: "security",
    title: "Security",
    body: [
      "We use technical and organisational measures designed to protect Hivy, including TLS in transit, encryption for sensitive credentials, access controls, audit logging, monitoring, secret handling, sandbox isolation, least privilege practices, and internal access restrictions.",
      "No system is perfectly secure. You are responsible for protecting your passwords, devices, API keys, connected service accounts, workspace roles, and integration scopes.",
      "If you believe your account, workspace, integration, or data has been compromised, contact hello@usehivy.com.",
    ],
  },
  {
    id: "cookies",
    title: "Cookies",
    body: [
      "We use cookies and similar technologies for authentication, session management, security, preferences, analytics, performance, and diagnostics.",
      "Essential cookies are needed for Hivy to work. Optional analytics or marketing cookies will be used only where permitted by law and subject to consent or available controls where required.",
      "You can control cookies through your browser settings, but blocking essential cookies may prevent Hivy from working correctly. Hivy does not use analytics or marketing cookies at launch.",
    ],
  },
  {
    id: "children",
    title: "Children",
    body: [
      "Hivy is not intended for children and must not be used by anyone under 18.",
      "Customer Data may contain information about children if a workspace submits it. Workspace owners must not submit children's personal data unless they have the required authority, lawful basis, consent where required, notices, and safeguards.",
    ],
  },
  {
    id: "lawful-requests",
    title: "Lawful requests",
    body: [
      "We may preserve, disclose, or provide access to data where required by Nigerian law, court order, regulator request, authorised government agency request, cybercrime investigation, tax requirement, or other legally valid process.",
      "Where legally allowed and practical, we will notify affected customers or users before disclosing Customer Data.",
    ],
  },
  {
    id: "contact",
    title: "Contact",
    body: [
      "HIVY TECHNOLOGIES LTD, registered number 1078455345.",
      "Registered office: B1 Orji Murray St, behind Romay Garden Estate, off Lekki - Epe Expressway, Eti-Osa, Lekki 106104, Lagos.",
      "Privacy requests, legal notices, and security reports: hello@usehivy.com.",
    ],
  },
  {
    id: "changes",
    title: "Changes",
    body: [
      "We may update this Legal page as Hivy, our providers, our data practices, or applicable law change. If changes are material, we will take reasonable steps to notify users through the product, email, website, or another appropriate channel.",
    ],
  },
]

export default function LegalPageRoot() {
  return (
    <LegalPage
      title="Legal"
      eyebrow="Legal"
      effectiveDate="May 28, 2026"
      lastUpdated="May 28, 2026"
      version="1"
      sections={sections}
    />
  )
}
