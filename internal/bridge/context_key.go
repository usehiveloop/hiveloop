package bridge

// bearerContextKey is the context key referenced by the generated client's
// SecurityScheme constants. oapi-codegen with `generate.models: true` no
// longer emits this helper; declare it here so client_gen.go compiles.
type bearerContextKey string
