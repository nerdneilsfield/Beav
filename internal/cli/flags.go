package cli

type CleanFlags struct {
	System          bool
	All             bool
	DryRun          bool
	Only            []string
	Skip            []string
	MinAge          string
	ForceNoAge      bool
	Output          string
	Yes             bool
	AllowRootHome   bool
	UserOverride    string
	ConfigDir       string
	BuiltinDisabled bool
}
