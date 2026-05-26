export type TemplateVariable = {
  key: string
  type: "string" | "number" | "boolean"
}

export type TemplateDefinition = {
  alias: string
  name: string
  subject: string
  variables: TemplateVariable[]
}

const stringVars = (keys: string[]): TemplateVariable[] =>
  keys.map((key) => ({ key, type: "string" }))

export const templates: TemplateDefinition[] = [
  {
    alias: "auth-confirm-email",
    name: "Auth - Confirm email",
    subject: "Your Hivy confirmation code: {{{code}}}",
    variables: stringVars(["code", "email", "expiresIn", "firstName"]),
  },
  {
    alias: "auth-otp-login",
    name: "Auth - OTP login code",
    subject: "Your Hivy login code: {{{code}}}",
    variables: stringVars(["code", "email", "expiresIn"]),
  },
  {
    alias: "auth-password-changed",
    name: "Auth - Password changed",
    subject: "Your Hivy password was changed",
    variables: stringVars(["changedAt", "firstName"]),
  },
  {
    alias: "auth-password-reset",
    name: "Auth - Password reset",
    subject: "Reset your Hivy password",
    variables: stringVars(["expiresIn", "firstName", "resetUrl"]),
  },
  {
    alias: "auth-welcome",
    name: "Auth - Welcome",
    subject: "Welcome to Hivy, {{{firstName}}}",
    variables: stringVars(["firstName"]),
  },
  {
    alias: "org-invite",
    name: "Org - Invitation to join",
    subject: "{{{inviterName}}} invited you to {{{orgName}}}",
    variables: stringVars([
      "expiresIn",
      "firstName",
      "inviteUrl",
      "inviterName",
      "orgName",
      "role",
    ]),
  },
  {
    alias: "org-invite-accepted",
    name: "Org - Invite accepted",
    subject: "{{{invitedName}}} joined {{{orgName}}}",
    variables: stringVars([
      "adminFirstName",
      "invitedEmail",
      "invitedName",
      "orgName",
      "role",
    ]),
  },
]
