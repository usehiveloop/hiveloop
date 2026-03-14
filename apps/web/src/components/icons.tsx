export function LockIcon({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 108 108" fill="none">
      <rect x="4" y="4" width="100" height="100" stroke="#8B5CF6" strokeWidth="8" fill="none" />
      <rect x="34" y="48" width="40" height="32" fill="#8B5CF6" />
      <path d="M 42 48 L 42 30 L 66 30 L 66 48" stroke="#8B5CF6" strokeWidth="7" strokeLinecap="square" fill="none" />
    </svg>
  );
}

export function CheckIcon({ className }: { className?: string }) {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" className={className}>
      <path d="M3 8L6.5 11.5L13 4.5" stroke="#8B5CF6" strokeWidth="2" strokeLinecap="square" />
    </svg>
  );
}
