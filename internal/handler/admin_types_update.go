package handler

type adminUpdateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
}

type adminUpdateOrgRequest struct {
	Name           *string   `json:"name,omitempty"`
	RateLimit      *int      `json:"rate_limit,omitempty"`
	Active         *bool     `json:"active,omitempty"`
	AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
}

type adminUpdateCredentialRequest struct {
	Label *string `json:"label,omitempty"`
}

type adminCreateSandboxTemplateRequest struct {
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	Slug         string   `json:"slug"`
	Tags         []string `json:"tags"`
	Size         string   `json:"size"`
	BaseImageRef *string  `json:"base_image_ref,omitempty"`
}

type adminUpdateSandboxTemplateRequest struct {
	Name         *string  `json:"name,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Slug         *string  `json:"slug,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Size         *string  `json:"size,omitempty"`
	BaseImageRef *string  `json:"base_image_ref,omitempty"`
}
