# Hivy Email Templates

React Email workspace for Hivy transactional emails.

## Workflow

1. Add approved React Email components to `emails/`.
2. Preview locally with `pnpm dev`.
3. Dry-run the production upload with `pnpm upload:resend:dry`.
4. Upload and publish with `RESEND_API_KEY=... pnpm upload:resend`.

Local preview serves logo images from `emails/static`. When uploading templates to
Resend, the custom uploader sets `HIVY_EMAIL_RENDER_TARGET=resend` so rendered
HTML uses production image URLs from `https://usehivy.com/email`. Override the
public asset host with `HIVY_EMAIL_ASSET_URL=https://your-host.example/email pnpm upload:resend`.

## Production Upload

Use the custom uploader when templates need to be sent by backend alias. The React
Email UI upload does not declare Resend variables, aliases, or subjects.

```bash
RESEND_API_KEY=... pnpm upload:resend
```

Dry-run without touching Resend:

```bash
pnpm upload:resend:dry
```

The uploader renders templates with `{{{variable}}}` placeholders, creates or
updates each Resend template by alias, declares variables from `templates.ts`,
sets the subject, and publishes the template.

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
