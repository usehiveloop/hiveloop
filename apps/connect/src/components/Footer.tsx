/** Exact replica of Paper "Footer" node — appears on every screen.
 *  Light: logo=#8B5CF6, "Secured by"=#A1A1AA, "ziraloop"=#09090B
 *  Dark:  logo=#A78BFA, "Secured by"=#71717A, "ziraloop"=#F5F5F4
 */
export function Footer() {
  return (
    <div className="absolute bottom-6 inset-x-0 flex items-center justify-center gap-1.5">
      <div className="flex items-center gap-1.5">
        <div className="text-2xs leading-3.5 text-cw-secondary">
          Secured by
        </div>
        <div className="flex items-center gap-1">
          <svg width="14" height="14" viewBox="0 0 108 108" fill="none">
            <rect x="4" y="4" width="100" height="100" stroke="var(--color-cw-logo)" strokeWidth="8" fill="none" />
            <rect x="34" y="48" width="40" height="32" fill="var(--color-cw-logo)" />
            <path d="M42 48L42 30L66 30L66 48" stroke="var(--color-cw-logo)" strokeWidth="7" strokeLinecap="square" fill="none" />
          </svg>
          <div className="text-xs tracking-tight leading-3.5 text-cw-heading connect-logo-text font-semibold">
            ziraloop
          </div>
        </div>
      </div>
    </div>
  )
}
