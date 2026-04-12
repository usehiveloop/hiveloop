package dispatch

// ContextRequest is a fully-resolved read action to execute for gathering
// context. Built by buildContextRequests from ContextAction definitions.
// The executor resolves {{$step.x}} deferred variables after each fetch.
type ContextRequest struct {
	As           string            // context bag key, e.g. "issue", "files"
	ActionKey    string            // catalog action key, e.g. "issues_get"
	Method       string            // HTTP method copied from action.Execution.Method
	Path         string            // already substituted with refs (e.g. /repos/octocat/Hello-World/issues/1347)
	Query        map[string]string // resolved query params (may contain {{$step.x}})
	Body         map[string]any    // resolved body params (may contain {{$step.x}})
	Headers      map[string]string // copied from action.Execution.Headers
	Optional     bool              // failure does not block the run
	DeferredVars []string          // {{$step.x}} placeholders found in this request's params
}
