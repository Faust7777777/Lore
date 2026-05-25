package cli

import (
	"flag"
	"strings"
)

// reorderFlagsBeforePositionals shuffles args so every recognized
// flag (and its value, for non-boolean flags) appears before any
// positional argument.
//
// Go's stdlib `flag.FlagSet.Parse` stops processing flags at the
// first positional argument, so `lore persona candidates show pc-1
// --json` would treat `--json` as an unknown positional rather than
// setting the boolean. Reordering before Parse lets the operator
// write flags after the ID the way `git`, `kubectl`, and most
// modern CLIs do, without pulling in a POSIX-style flag library.
//
// flagSet MUST have all flags defined before this is called; the
// reorder logic uses `flags.Lookup(name)` + the optional
// IsBoolFlag interface to decide whether the next arg is the
// flag's value or a positional. Unknown flag-shaped args are
// passed through to the front so flag.Parse can produce its
// normal "unknown flag" error.
func reorderFlagsBeforePositionals(args []string, flagSet *flag.FlagSet) []string {
	var flagArgs, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Standard "--" terminator: everything after it is positional.
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		// Strip leading dashes to get the flag name (with optional
		// =value suffix attached).
		name := strings.TrimLeft(arg, "-")
		eq := strings.IndexByte(name, '=')
		explicitValue := eq >= 0
		if explicitValue {
			name = name[:eq]
		}
		f := flagSet.Lookup(name)
		if f == nil {
			// Unknown flag; let flag.Parse report the error in its
			// usual format. Push the arg as-is to the flag side so
			// Parse sees it before any positional.
			flagArgs = append(flagArgs, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		if explicitValue {
			continue
		}
		// Bool flags do not consume the next arg; value flags do.
		if isBoolFlag(f) {
			continue
		}
		if i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return append(flagArgs, positionals...)
}

func isBoolFlag(f *flag.Flag) bool {
	if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
		return bf.IsBoolFlag()
	}
	return false
}
