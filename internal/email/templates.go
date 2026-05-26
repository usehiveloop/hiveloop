package email

import "strings"

// TemplateSlug is the Resend template alias for a transactional email.
type TemplateSlug string

// TemplateVars are stringified template substitutions.
type TemplateVars = map[string]string

const (
	TmplAuthConfirmEmail    TemplateSlug = "auth-confirm-email"
	TmplAuthOtpLogin        TemplateSlug = "auth-otp-login"
	TmplAuthPasswordChanged TemplateSlug = "auth-password-changed"
	TmplAuthPasswordReset   TemplateSlug = "auth-password-reset"
	TmplAuthWelcome         TemplateSlug = "auth-welcome"
	TmplOrgInvite           TemplateSlug = "org-invite"
	TmplOrgInviteAccepted   TemplateSlug = "org-invite-accepted"
)

type TemplateMeta struct {
	Slug        TemplateSlug
	Name        string
	Subject     string
	Description string
	Variables   []string
}

var templateMeta = map[TemplateSlug]TemplateMeta{
	TmplAuthConfirmEmail: {
		Slug:        TmplAuthConfirmEmail,
		Name:        "Auth - Confirm email",
		Subject:     "Your Hivy confirmation code: {{code}}",
		Description: "Sent on signup or when re-sending an email confirmation code.",
		Variables:   []string{"code", "email", "expiresIn", "firstName"},
	},
	TmplAuthOtpLogin: {
		Slug:        TmplAuthOtpLogin,
		Name:        "Auth - OTP login code",
		Subject:     "Your Hivy login code: {{code}}",
		Description: "Sent when a user requests a passwordless email login.",
		Variables:   []string{"code", "email", "expiresIn"},
	},
	TmplAuthPasswordChanged: {
		Slug:        TmplAuthPasswordChanged,
		Name:        "Auth - Password changed",
		Subject:     "Your Hivy password was changed",
		Description: "Sent after a successful password reset or password change.",
		Variables:   []string{"changedAt", "firstName"},
	},
	TmplAuthPasswordReset: {
		Slug:        TmplAuthPasswordReset,
		Name:        "Auth - Password reset",
		Subject:     "Reset your Hivy password",
		Description: "Sent when a user requests a password reset.",
		Variables:   []string{"expiresIn", "firstName", "resetUrl"},
	},
	TmplAuthWelcome: {
		Slug:        TmplAuthWelcome,
		Name:        "Auth - Welcome",
		Subject:     "Welcome to Hivy, {{firstName}}",
		Description: "Sent after a user confirms their email for the first time.",
		Variables:   []string{"firstName"},
	},
	TmplOrgInvite: {
		Slug:        TmplOrgInvite,
		Name:        "Org - Invitation to join",
		Subject:     "{{inviterName}} invited you to {{orgName}}",
		Description: "Sent when an admin invites a user to an organization.",
		Variables:   []string{"expiresIn", "firstName", "inviteUrl", "inviterName", "orgName", "role"},
	},
	TmplOrgInviteAccepted: {
		Slug:        TmplOrgInviteAccepted,
		Name:        "Org - Invite accepted",
		Subject:     "{{invitedName}} joined {{orgName}}",
		Description: "Notifies the inviter when an invitation is accepted.",
		Variables:   []string{"adminFirstName", "invitedEmail", "invitedName", "orgName", "role"},
	},
}

func (s TemplateSlug) VariablesFor() []string {
	m, ok := templateMeta[s]
	if !ok {
		return nil
	}
	out := make([]string, len(m.Variables))
	copy(out, m.Variables)
	return out
}

func (s TemplateSlug) Meta() (TemplateMeta, bool) {
	m, ok := templateMeta[s]
	return m, ok
}

func AllSlugs() []TemplateSlug {
	out := make([]TemplateSlug, 0, len(templateMeta))
	for s := range templateMeta {
		out = append(out, s)
	}
	return out
}

func Validate(slug TemplateSlug, vars TemplateVars) []string {
	required := slug.VariablesFor()
	missing := make([]string, 0, len(required))
	for _, k := range required {
		v, ok := vars[k]
		if !ok || v == "" {
			missing = append(missing, k)
		}
	}
	return missing
}

func Subject(slug TemplateSlug, vars TemplateVars) string {
	meta, ok := slug.Meta()
	if !ok {
		return string(slug)
	}
	subject := meta.Subject
	for k, v := range vars {
		subject = strings.ReplaceAll(subject, "{{"+k+"}}", v)
		subject = strings.ReplaceAll(subject, "{{{"+k+"}}}", v)
	}
	return subject
}
