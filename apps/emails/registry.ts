import type { ComponentType } from "react"

import AuthConfirmEmail, { type AuthConfirmEmailProps } from "./emails/auth-confirm-email"
import AuthOtpLogin, { type AuthOtpLoginProps } from "./emails/auth-otp-login"
import AuthPasswordChanged, {
  type AuthPasswordChangedProps,
} from "./emails/auth-password-changed"
import AuthPasswordReset, { type AuthPasswordResetProps } from "./emails/auth-password-reset"
import AuthWelcome, { type AuthWelcomeProps } from "./emails/auth-welcome"
import OrgInvite, { type OrgInviteProps } from "./emails/org-invite"
import OrgInviteAccepted, { type OrgInviteAcceptedProps } from "./emails/org-invite-accepted"
import { templates, type TemplateDefinition } from "./templates"

type RegistryEntry<Props extends Record<string, string>> = {
  definition: TemplateDefinition
  component: ComponentType<Props>
  placeholderProps: Props
}

const byAlias = new Map(templates.map((template) => [template.alias, template]))

function definition(alias: string) {
  const found = byAlias.get(alias)
  if (!found) throw new Error(`Missing template definition for ${alias}`)
  return found
}

const variable = (key: string) => `{{{${key}}}}`

export const templateRegistry = [
  {
    definition: definition("auth-confirm-email"),
    component: AuthConfirmEmail,
    placeholderProps: {
      code: variable("code"),
      email: variable("email"),
      expiresIn: variable("expiresIn"),
      firstName: variable("firstName"),
    } satisfies AuthConfirmEmailProps,
  },
  {
    definition: definition("auth-otp-login"),
    component: AuthOtpLogin,
    placeholderProps: {
      code: variable("code"),
      email: variable("email"),
      expiresIn: variable("expiresIn"),
    } satisfies AuthOtpLoginProps,
  },
  {
    definition: definition("auth-password-changed"),
    component: AuthPasswordChanged,
    placeholderProps: {
      changedAt: variable("changedAt"),
      firstName: variable("firstName"),
    } satisfies AuthPasswordChangedProps,
  },
  {
    definition: definition("auth-password-reset"),
    component: AuthPasswordReset,
    placeholderProps: {
      expiresIn: variable("expiresIn"),
      firstName: variable("firstName"),
      resetUrl: variable("resetUrl"),
    } satisfies AuthPasswordResetProps,
  },
  {
    definition: definition("auth-welcome"),
    component: AuthWelcome,
    placeholderProps: {
      firstName: variable("firstName"),
    } satisfies AuthWelcomeProps,
  },
  {
    definition: definition("org-invite"),
    component: OrgInvite,
    placeholderProps: {
      expiresIn: variable("expiresIn"),
      firstName: variable("firstName"),
      inviteUrl: variable("inviteUrl"),
      inviterName: variable("inviterName"),
      orgName: variable("orgName"),
      role: variable("role"),
    } satisfies OrgInviteProps,
  },
  {
    definition: definition("org-invite-accepted"),
    component: OrgInviteAccepted,
    placeholderProps: {
      adminFirstName: variable("adminFirstName"),
      invitedEmail: variable("invitedEmail"),
      invitedName: variable("invitedName"),
      orgName: variable("orgName"),
      role: variable("role"),
    } satisfies OrgInviteAcceptedProps,
  },
] satisfies RegistryEntry<any>[]
