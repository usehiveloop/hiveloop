export function RemainingBar({
  current,
  max,
  percent,
}: {
  current: string;
  max: string;
  percent: number;
}) {
  return (
    <div className="flex flex-col gap-1 pr-4">
      <div className="flex items-center gap-1.5">
        <div className="h-1 flex-1 bg-secondary">
          <div className="h-full bg-primary" style={{ width: `${percent}%` }} />
        </div>
        <span className="font-mono text-[11px] text-muted-foreground">{current}</span>
      </div>
      <span className="text-2xs text-[#6B6B75]">of {max}</span>
    </div>
  );
}

export function RemainingBarCompact({
  current,
  max,
  percent,
}: {
  current: string;
  max: string;
  percent: number;
}) {
  return (
    <div className="flex items-center gap-2">
      <div className="h-1 w-16 bg-secondary">
        <div className="h-full bg-primary" style={{ width: `${percent}%` }} />
      </div>
      <span className="font-mono text-[11px]">
        {current} / {max}
      </span>
    </div>
  );
}
