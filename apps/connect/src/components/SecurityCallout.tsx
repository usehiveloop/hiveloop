import { ShieldIcon } from './icons'

export function SecurityCallout() {
  return (
    <>
      {/* Desktop */}
      <div className="cw-mobile:hidden flex items-start mt-2 shrink-0 rounded-lg gap-2.5 bg-cw-accent-subtle border border-solid border-cw-accent-subtle-border p-3.5">
        <ShieldIcon size={18} className="shrink-0 mt-px" />
        <div className="text-[13px] leading-normal text-cw-body">
          Your key is encrypted end-to-end with AES-256-GCM and never stored in plaintext.
        </div>
      </div>
      {/* Mobile */}
      <div className="cw-desktop:hidden flex items-start mt-2 shrink-0 rounded-2.5 gap-2.5 bg-cw-surface p-3.5">
        <ShieldIcon size={16} className="shrink-0 mt-px" />
        <div className="flex flex-col gap-0.5">
          <div className="text-[13px] text-cw-heading font-medium leading-4">
            End-to-end encrypted
          </div>
          <div className="text-xs text-cw-secondary leading-4">
            Your key is encrypted with AES-256 before leaving this device.
          </div>
        </div>
      </div>
    </>
  )
}
