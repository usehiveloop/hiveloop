import Link from "next/link";

type DocLink = { label: string; href: string };

export function DocsBreadcrumb({ items }: { items: string[] }) {
  return (
    <div className="flex items-center gap-1.5">
      {items.map((item, i) => (
        <span key={i} className="flex items-center gap-1.5">
          {i > 0 && (
            <span className="text-[13px] leading-4 text-[#575560]">/</span>
          )}
          <span
            className={`text-[13px] leading-4 ${
              i === items.length - 1 ? "text-[#E4E1EC]" : "text-[#9794A3]"
            }`}
          >
            {item}
          </span>
        </span>
      ))}
    </div>
  );
}

export function DocsPageHeader({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-mono text-4xl font-medium leading-11 text-[#E4E1EC]">
        {title}
      </h1>
      <p className="text-[17px] leading-7 text-[#9794A3]">{description}</p>
    </div>
  );
}

export function DocsPrevNext({
  prev,
  next,
}: {
  prev?: DocLink;
  next?: DocLink;
}) {
  return (
    <div className="flex justify-between border-t border-border pt-8">
      {prev ? (
        <Link href={prev.href} className="flex flex-col gap-1">
          <span className="text-xs leading-4 text-[#9794A3]">Previous</span>
          <span className="text-sm leading-4.5 text-[#A78BFA]">
            &larr; {prev.label}
          </span>
        </Link>
      ) : (
        <div />
      )}
      {next ? (
        <Link href={next.href} className="flex flex-col items-end gap-1">
          <span className="text-xs leading-4 text-[#9794A3]">Next</span>
          <span className="text-sm leading-4.5 text-[#A78BFA]">
            {next.label} &rarr;
          </span>
        </Link>
      ) : (
        <div />
      )}
    </div>
  );
}

export function DocsDivider() {
  return <div className="h-px w-full shrink-0 bg-border" />;
}
