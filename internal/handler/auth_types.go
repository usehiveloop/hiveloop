package handler

import (


)
// --- Request / Response types ---

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id,omitempty"` // optional: scope token to a specific org
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	OrgID        string `json:"org_id,omitempty"` // optional: switch org
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int            `json:"expires_in"` // seconds
	User         userResponse   `json:"user"`
	Orgs         []orgMemberDTO `json:"orgs"`
	// Impersonation is true when the tokens were issued by a platform admin
	// impersonating the user. Clients can surface a visible banner when set.
	Impersonation bool `json:"impersonation,omitempty"`
	// ImpersonationOf is the admin user ID that initiated the session, when
	// Impersonation is true. Empty for regular sessions.
	ImpersonationOf string `json:"impersonation_of,omitempty"`
}

type userResponse struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	EmailConfirmed bool   `json:"email_confirmed"`
}

type orgMemberDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type meResponse struct {
	User            userResponse   `json:"user"`
	Orgs            []orgMemberDTO `json:"orgs"`
	IsPlatformAdmin bool           `json:"is_platform_admin"`
}

type statusResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
