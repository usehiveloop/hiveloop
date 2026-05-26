# Hivy Email Templates

React Email workspace for Hivy transactional emails.

## Workflow

1. Add approved React Email components to `emails/`.
2. Preview locally with `pnpm dev`.
3. Configure Resend upload with `pnpm exec email resend setup`.
4. Upload with production image URLs using `pnpm dev:resend`.
5. Use the React Email UI `Resend` tab to upload one template or bulk upload all templates.

Local preview serves logo images from `emails/static`. When uploading templates to
Resend, `pnpm dev:resend` sets `HIVY_EMAIL_RENDER_TARGET=resend` so rendered HTML
uses production image URLs from `https://usehivy.com/email`. Override the public
asset host with `HIVY_EMAIL_ASSET_URL=https://your-host.example/email pnpm dev:resend`.

The backend sends by the aliases listed in `templates.ts`; keep that manifest in sync with `internal/email/templates.go`.

## Design Approval

The seven live templates are:

- `auth-confirm-email`
- `auth-otp-login`
- `auth-password-changed`
- `auth-password-reset`
- `auth-welcome`
- `org-invite`
- `org-invite-accepted`

Local preview uses realistic sample data.
