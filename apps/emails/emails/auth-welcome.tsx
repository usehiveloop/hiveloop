import { HivyEmail, Paragraph, PrimaryButton, siteUrl } from "../lib/hivy-email"

export type AuthWelcomeProps = {
  firstName: string
}

export default function AuthWelcome({ firstName }: AuthWelcomeProps) {
  return (
    <HivyEmail
      preview="Your Hivy workspace is ready"
      eyebrow="Welcome to Hivy"
      title={`You're in, ${firstName}`}
    >
      <Paragraph>
        Your email is confirmed. Hivy is ready to help your workspace connect tools, organize context,
        and move work forward from one place.
      </Paragraph>
      <PrimaryButton href={`${siteUrl}/w`}>Open Hivy</PrimaryButton>
      <Paragraph>
        Start by connecting the tools your team already uses, then give Hivy the context it needs to
        help with day-to-day work.
      </Paragraph>
    </HivyEmail>
  )
}

AuthWelcome.PreviewProps = {
  firstName: "Ada",
} satisfies AuthWelcomeProps
