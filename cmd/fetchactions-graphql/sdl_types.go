package main

// sdlSchema holds the parsed Query and Mutation fields from an SDL file.
type sdlSchema struct {
	QueryFields    []sdlField
	MutationFields []sdlField
	InputTypes     map[string][]sdlField
	ObjectTypes    map[string][]sdlField
}

// sdlField represents a single field on a Query, Mutation, input, or object type.
type sdlField struct {
	Name        string
	Description string
	Args        []sdlArg
	Type        string
}

// sdlArg represents an argument on a field.
type sdlArg struct {
	Name        string
	Type        string
	Description string
	Required    bool
}
