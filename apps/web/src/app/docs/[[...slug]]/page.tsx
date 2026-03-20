import {
  DocsBreadcrumb,
  DocsPageHeader,
  DocsPrevNext,
  DocsDivider,
} from "@/components/docs-nav";
import { DocsContent, type TocItem } from "./content";
import { getDocContent, extractToc } from "@/lib/markdown";

type DocMeta = {
  title: string;
  description: string;
  breadcrumb: string[];
  section: string;
  prev?: { label: string; href: string };
  next?: { label: string; href: string };
};

// Navigation structure for prev/next links
const docNavOrder = [
  // Getting Started
  { slug: ["getting-started", "introduction"], section: "Getting Started" },
  { slug: ["getting-started", "quickstart"], section: "Getting Started" },
  { slug: ["getting-started", "installation"], section: "Getting Started" },
  { slug: ["getting-started", "authentication"], section: "Getting Started" },
  // Core Concepts
  { slug: ["core-concepts", "credentials"], section: "Core Concepts" },
  { slug: ["core-concepts", "tokens"], section: "Core Concepts" },
  { slug: ["core-concepts", "proxy"], section: "Core Concepts" },
  { slug: ["core-concepts", "providers"], section: "Core Concepts" },
  { slug: ["core-concepts", "identities"], section: "Core Concepts" },
  { slug: ["core-concepts", "rate-limiting"], section: "Core Concepts" },
  // Connect
  { slug: ["connect", "overview"], section: "Connect" },
  { slug: ["connect", "embedding"], section: "Connect" },
  { slug: ["connect", "theming"], section: "Connect" },
  { slug: ["connect", "sessions"], section: "Connect" },
  { slug: ["connect", "providers"], section: "Connect" },
  { slug: ["connect", "integrations"], section: "Connect" },
  { slug: ["connect", "frontend-sdk"], section: "Connect" },
  // Dashboard
  { slug: ["dashboard", "overview"], section: "Dashboard" },
  { slug: ["dashboard", "credentials"], section: "Dashboard" },
  { slug: ["dashboard", "tokens"], section: "Dashboard" },
  { slug: ["dashboard", "api-keys"], section: "Dashboard" },
  { slug: ["dashboard", "identities"], section: "Dashboard" },
  { slug: ["dashboard", "integrations"], section: "Dashboard" },
  { slug: ["dashboard", "audit-log"], section: "Dashboard" },
  { slug: ["dashboard", "team"], section: "Dashboard" },
  { slug: ["dashboard", "billing"], section: "Dashboard" },
  // SDKs
  { slug: ["sdks", "typescript"], section: "SDKs" },
  { slug: ["sdks", "python"], section: "SDKs" },
  { slug: ["sdks", "go"], section: "SDKs" },
  { slug: ["sdks", "frontend"], section: "SDKs" },
  // Security
  { slug: ["security", "overview"], section: "Security" },
  { slug: ["security", "encryption"], section: "Security" },
  { slug: ["security", "token-scoping"], section: "Security" },
  { slug: ["security", "audit-logging"], section: "Security" },
  { slug: ["security", "compliance"], section: "Security" },
  // Self-Hosting
  { slug: ["self-hosting", "overview"], section: "Self-Hosting" },
  { slug: ["self-hosting", "docker-compose"], section: "Self-Hosting" },
  { slug: ["self-hosting", "kubernetes"], section: "Self-Hosting" },
  { slug: ["self-hosting", "configuration"], section: "Self-Hosting" },
  { slug: ["self-hosting", "environment"], section: "Self-Hosting" },
];

function findDocIndex(slug: string[]): number {
  const slugStr = slug.join("/");
  return docNavOrder.findIndex((doc) => doc.slug.join("/") === slugStr);
}

function getPrevNext(
  slug: string[]
): { prev?: { label: string; href: string }; next?: { label: string; href: string } } {
  const currentIndex = findDocIndex(slug);
  if (currentIndex === -1) return {};

  const prev =
    currentIndex > 0
      ? {
          label: docNavOrder[currentIndex - 1].slug[docNavOrder[currentIndex - 1].slug.length - 1]
            .split("-")
            .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
            .join(" "),
          href: `/docs/${docNavOrder[currentIndex - 1].slug.join("/")}`,
        }
      : undefined;

  const next =
    currentIndex < docNavOrder.length - 1
      ? {
          label: docNavOrder[currentIndex + 1].slug[docNavOrder[currentIndex + 1].slug.length - 1]
            .split("-")
            .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
            .join(" "),
          href: `/docs/${docNavOrder[currentIndex + 1].slug.join("/")}`,
        }
      : undefined;

  return { prev, next };
}

function formatBreadcrumb(
  slug: string[],
  frontmatter: { title: string }
): string[] {
  const index = findDocIndex(slug);
  if (index === -1) return ["Docs", frontmatter.title];

  const section = docNavOrder[index].section;
  return ["Docs", section, frontmatter.title];
}

export default async function DocsPage({
  params,
}: {
  params: Promise<{ slug?: string[] }>;
}) {
  const { slug } = await params;
  const slugPath = slug || ["getting-started", "introduction"];

  const doc = await getDocContent(slugPath);

  if (!doc) {
    return (
      <div className="flex flex-col gap-8">
        <DocsPageHeader
          title="Not Found"
          description="This documentation page doesn't exist yet."
        />
      </div>
    );
  }

  const toc: TocItem[] = extractToc(doc.content);
  const { prev, next } = getPrevNext(slugPath);
  const breadcrumb = formatBreadcrumb(slugPath, doc.frontmatter);

  return (
    <div className="flex flex-col gap-8">
      <DocsBreadcrumb items={breadcrumb} />
      <DocsPageHeader
        title={doc.frontmatter.title}
        description={doc.frontmatter.description || ""}
      />
      <DocsDivider />
      <DocsContent content={doc.content} toc={toc} />
      <DocsPrevNext prev={prev} next={next} />
    </div>
  );
}
