import { CodePanel, Divider, HivyEmail, Paragraph } from "../lib/hivy-email"

export type AuthConfirmEmailProps = {
  code: string
  email: string
  expiresIn: string
  firstName: string
}

export default function AuthConfirmEmail({
  code,
  email,
  expiresIn,
  firstName,
}: AuthConfirmEmailProps) {
  return (
    <HivyEmail
      preview={`Confirm ${email} for Hivy`}
      eyebrow="Confirm your email"
      title={`Finish setting up Hivy, ${firstName}`}
      footerNote={`This confirmation was sent to ${email}.`}
    >
      <Paragraph>Use this code to confirm your email address and protect your Hivy account.</Paragraph>
      <CodePanel code={code} />
      <Paragraph>The code expires in {expiresIn}. After confirmation, you can keep working in Hivy.</Paragraph>
      <Divider />
      <Paragraph>If you did not create or update a Hivy account, no action is needed.</Paragraph>
    </HivyEmail>
  )
}

AuthConfirmEmail.PreviewProps = {
  code: "629104",
  email: "ada@example.com",
  expiresIn: "10 minutes",
  firstName: "Ada",
} satisfies AuthConfirmEmailProps
