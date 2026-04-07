package spider

// SpiderParams covers common parameters for crawl, links, and screenshot endpoints.
// Maps to the Spider.cloud POST body for /v1/crawl, /v1/links, /v1/screenshot.
type SpiderParams struct {
	URL             string            `json:"url"`
	Limit           *int              `json:"limit,omitempty"`
	Depth           *int              `json:"depth,omitempty"`
	ReturnFormat    string            `json:"return_format,omitempty"`    // "markdown", "raw", "text", "html2text", "bytes", "empty"
	RequestType     string            `json:"request,omitempty"`          // "http", "chrome", "smart"
	Readability     *bool             `json:"readability,omitempty"`
	Cache           *bool             `json:"cache,omitempty"`
	Stealth         *bool             `json:"stealth,omitempty"`
	Storageless     *bool             `json:"storageless,omitempty"`
	TLD             *bool             `json:"tld,omitempty"`
	Subdomains      *bool             `json:"subdomains,omitempty"`
	Metadata        *bool             `json:"metadata,omitempty"`
	RespectRobots   *bool             `json:"respect_robots,omitempty"`
	FullResources   *bool             `json:"full_resources,omitempty"`
	ReturnPageLinks *bool             `json:"return_page_links,omitempty"`
	ReturnHeaders   *bool             `json:"return_headers,omitempty"`
	ReturnCookies   *bool             `json:"return_cookies,omitempty"`
	Sitemap         *bool             `json:"sitemap,omitempty"`
	RequestTimeout  *int              `json:"request_timeout,omitempty"`
	Delay           *int              `json:"delay,omitempty"`
	Scroll          *int              `json:"scroll,omitempty"`
	RedirectLimit   *int              `json:"redirect_limit,omitempty"`
	Blacklist       []string          `json:"blacklist,omitempty"`
	Whitelist       []string          `json:"whitelist,omitempty"`
	ExternalDomains []string          `json:"external_domains,omitempty"`
	Budget          map[string]int    `json:"budget,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	UserAgent       string            `json:"user_agent,omitempty"`
	Proxy           string            `json:"proxy,omitempty"`
	CountryCode     string            `json:"country_code,omitempty"`
	Locale          string            `json:"locale,omitempty"`
}

// SearchParams extends SpiderParams with search-specific fields for /v1/search.
type SearchParams struct {
	SpiderParams
	Search           string `json:"search"`
	SearchLimit      *int   `json:"search_limit,omitempty"`
	FetchPageContent *bool  `json:"fetch_page_content,omitempty"`
	Country          string `json:"country,omitempty"`
	Language         string `json:"language,omitempty"`
	Num              *int   `json:"num,omitempty"`
	Page             *int   `json:"page,omitempty"`
}

// TransformParams for /v1/transform (HTML → markdown without re-fetching).
type TransformParams struct {
	Data         []TransformInput `json:"data"`
	ReturnFormat string           `json:"return_format,omitempty"`
	Readability  *bool            `json:"readability,omitempty"`
}

// TransformInput is a single item to transform.
type TransformInput struct {
	HTML    string `json:"html,omitempty"`
	Content string `json:"content,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Response is a single result item from the Spider.cloud API.
// Fields map to the NPage struct from the spider-rs SDK:
//   url, content, status_code, raw_content, headers, links
type Response struct {
	// The URL of the page.
	URL string `json:"url,omitempty"`
	// The page content (HTML, markdown, text, etc. depending on return_format).
	Content string `json:"content,omitempty"`
	// The HTTP status code of the page fetch.
	StatusCode int `json:"status_code,omitempty"`
	// Raw binary content when rawContent mode is used. Base64-encoded in JSON.
	RawContent []byte `json:"raw_content,omitempty"`
	// HTTP response headers. Present when return_headers is true.
	Headers map[string]string `json:"headers,omitempty"`
	// Links found on the page. Present when return_page_links is true.
	Links []string `json:"links,omitempty"`
	// Error message if the page fetch failed.
	Error string `json:"error,omitempty"`
}
