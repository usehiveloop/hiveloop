import { Divider, HivyEmail, Paragraph, PrimaryButton } from "../lib/hivy-email"

export type AuthPasswordResetProps = {
  expiresIn: string
  firstName: string
  resetUrl: string
}

export default function AuthPasswordReset({
  expiresIn,
  firstName,
  resetUrl,
}: AuthPasswordResetProps) {
  return (
    <HivyEmail
      preview="Reset your Hivy password"
      eyebrow="Password reset"
      title={`${firstName}, reset your password`}
    >
      <Paragraph>
        We received a request to reset the password for your Hivy account. Use the button below to
        choose a new password.
      </Paragraph>
      <PrimaryButton href={resetUrl}>Reset password</PrimaryButton>
      <Paragraph>This link expires in {expiresIn}. If you did not request a reset, you can ignore this email.</Paragraph>
      <Divider />
      <Paragraph>For your security, Hivy support will never ask you to share this reset link.</Paragraph>
    </HivyEmail>
  )
}

AuthPasswordReset.PreviewProps = {
  expiresIn: "1 hour",
  firstName: "Ada",
  resetUrl: "https://usehivy.com/auth/reset-password?token=preview-token",
} satisfies AuthPasswordResetProps
