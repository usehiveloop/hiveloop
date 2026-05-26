import { Detail, HivyEmail, Paragraph, PrimaryButton, siteUrl } from "../lib/hivy-email"

export type OrgInviteAcceptedProps = {
  adminFirstName: string
  invitedEmail: string
  invitedName: string
  orgName: string
  role: string
}

export default function OrgInviteAccepted({
  adminFirstName,
  invitedEmail,
  invitedName,
  orgName,
  role,
}: OrgInviteAcceptedProps) {
  return (
    <HivyEmail
      preview={`${invitedName} joined ${orgName}`}
      eyebrow="Invitation accepted"
      title={`${adminFirstName}, ${invitedName} joined ${orgName}`}
    >
      <Paragraph>Your invitation was accepted and the new member is now part of the workspace.</Paragraph>
      <Detail label="Member" value={`${invitedName} (${invitedEmail})`} />
      <Detail label="Role" value={role} />
      <PrimaryButton href={`${siteUrl}/w/settings`}>Review workspace settings</PrimaryButton>
      <Paragraph>You can update roles and workspace access from Hivy settings.</Paragraph>
    </HivyEmail>
  )
}

OrgInviteAccepted.PreviewProps = {
  adminFirstName: "Ada",
  invitedEmail: "grace@example.com",
  invitedName: "Grace Hopper",
  orgName: "Analytical Engines Ltd",
  role: "member",
} satisfies OrgInviteAcceptedProps
