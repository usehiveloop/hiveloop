import { CodePanel, Divider, HivyEmail, Paragraph } from "../lib/hivy-email"

export type AuthOtpLoginProps = {
  code: string
  email: string
  expiresIn: string
}

export default function AuthOtpLogin({ code, email, expiresIn }: AuthOtpLoginProps) {
  return (
    <HivyEmail
      preview={`Your Hivy login code is ${code}`}
      eyebrow="Secure sign-in"
      title="Use this code to sign in"
      footerNote={`This code was requested for ${email}.`}
    >
      <Paragraph>Enter this code in Hivy to finish signing in to your workspace.</Paragraph>
      <CodePanel code={code} />
      <Paragraph>
        The code expires in {expiresIn}. If you did not request it, you can ignore this email.
      </Paragraph>
      <Divider />
      <Paragraph>Hivy will never ask for this code outside the sign-in flow.</Paragraph>
    </HivyEmail>
  )
}

AuthOtpLogin.PreviewProps = {
  code: "483921",
  email: "ada@example.com",
  expiresIn: "10 minutes",
} satisfies AuthOtpLoginProps
