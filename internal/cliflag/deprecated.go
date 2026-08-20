package cliflag

import (
	"flag"
	"fmt"
)

// StringVarWithDeprecatedAlias registers name as the canonical string flag and
// deprecatedName as a compatible alias that emits a migration warning when used.
func StringVarWithDeprecatedAlias(fs *flag.FlagSet, target *string, name, value, usage, deprecatedName string) {
	fs.StringVar(target, name, value, usage)
	fs.Var(&deprecatedStringValue{
		fs:             fs,
		target:         target,
		deprecatedName: deprecatedName,
		replacement:    name,
	}, deprecatedName, fmt.Sprintf("deprecated; use --%s", name))
}

type deprecatedStringValue struct {
	fs             *flag.FlagSet
	target         *string
	deprecatedName string
	replacement    string
}

func (v *deprecatedStringValue) String() string {
	if v == nil || v.target == nil {
		return ""
	}
	return *v.target
}

func (v *deprecatedStringValue) Set(value string) error {
	*v.target = value
	_, _ = fmt.Fprintf(v.fs.Output(), "warning: --%s is deprecated; use --%s instead\n", v.deprecatedName, v.replacement)
	return nil
}

func (v *deprecatedStringValue) Get() any {
	return *v.target
}
