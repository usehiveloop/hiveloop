import { Detail, Divider, HivyEmail, Paragraph, siteUrl } from "../lib/hivy-email"

export type AuthPasswordChangedProps = {
  changedAt: string
  firstName: string
}

export default function AuthPasswordChanged({ changedAt, firstName }: AuthPasswordChangedProps) {
  return (
    <HivyEmail
      preview="Your Hivy password was changed"
      eyebrow="Account security"
      title={`${firstName}, your password changed`}
    >
      <Paragraph>
        This is a confirmation that the password for your Hivy account was changed.
      </Paragraph>
      <Detail label="Changed at" value={changedAt} />
      <Paragraph>
        If this was you, no action is needed. If you do not recognize this change, reset your
        password and contact us immediately.
      </Paragraph>
      <Divider />
      <Paragraph>
        Visit {siteUrl}/legal for privacy, security, and legal contact details.
      </Paragraph>
    </HivyEmail>
  )
}

AuthPasswordChanged.PreviewProps = {
  changedAt: "Mon, 25 May 2026 21:30:00 UTC",
  firstName: "Ada",
} satisfies AuthPasswordChangedProps
