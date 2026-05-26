import { Detail, HivyEmail, Paragraph, PrimaryButton } from "../lib/hivy-email"

export type OrgInviteProps = {
  expiresIn: string
  firstName: string
  inviteUrl: string
  inviterName: string
  orgName: string
  role: string
}

export default function OrgInvite({
  expiresIn,
  firstName,
  inviteUrl,
  inviterName,
  orgName,
  role,
}: OrgInviteProps) {
  return (
    <HivyEmail
      preview={`${inviterName} invited you to ${orgName} on Hivy`}
      eyebrow="Workspace invitation"
      title={`${firstName}, join ${orgName}`}
    >
      <Paragraph>
        {inviterName} invited you to collaborate in {orgName} on Hivy.
      </Paragraph>
      <Detail label="Role" value={role} />
      <PrimaryButton href={inviteUrl}>Accept invitation</PrimaryButton>
      <Paragraph>
        This invitation expires in {expiresIn}. Hivy workspaces bring your team, connected tools, and
        AI employee context into one place.
      </Paragraph>
    </HivyEmail>
  )
}

OrgInvite.PreviewProps = {
  expiresIn: "7 days",
  firstName: "there",
  inviteUrl: "https://usehivy.com/invites/accept?token=preview-token",
  inviterName: "Ada Lovelace",
  orgName: "Analytical Engines Ltd",
  role: "admin",
} satisfies OrgInviteProps
