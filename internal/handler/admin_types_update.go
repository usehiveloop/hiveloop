package handler

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
