package cli

import (
	"flag"
	"reflect"
	"testing"
)

// newReorderFlagSet builds a FlagSet that mirrors the shape of the
// persona subcommands the reorder helper was written for: a couple
// of boolean flags, a couple of value flags, and an explicit
// SetOutput so the FlagSet does not spam stderr if Lookup is called
// for an undefined flag during the test.
func newReorderFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(discardWriter{})
	fs.Bool("json", false, "")
	fs.Bool("fail-on-orphan", false, "")
	fs.String("workdir", "", "")
	fs.String("state", "", "")
	fs.Int("limit", 0, "")
	return fs
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestReorderFlagsBeforePositionals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
		{
			name: "all positionals",
			in:   []string{"pc-1", "pc-2"},
			want: []string{"pc-1", "pc-2"},
		},
		{
			name: "all flags",
			in:   []string{"--json", "--workdir", "/wd"},
			want: []string{"--json", "--workdir", "/wd"},
		},
		{
			name: "bool flag after positional",
			in:   []string{"pc-1", "--json"},
			want: []string{"--json", "pc-1"},
		},
		{
			name: "value flag after positional pulls its value",
			in:   []string{"pc-1", "--workdir", "/wd"},
			want: []string{"--workdir", "/wd", "pc-1"},
		},
		{
			name: "interleaved flags and positionals",
			in:   []string{"--json", "pc-1", "--workdir", "/wd", "pc-2"},
			want: []string{"--json", "--workdir", "/wd", "pc-1", "pc-2"},
		},
		{
			name: "--name=value form does not consume next arg",
			in:   []string{"pc-1", "--workdir=/wd", "pc-2"},
			want: []string{"--workdir=/wd", "pc-1", "pc-2"},
		},
		{
			name: "double-dash terminator freezes following args as positionals",
			in:   []string{"--json", "--", "--workdir", "still-positional"},
			want: []string{"--json", "--workdir", "still-positional"},
		},
		{
			name: "single dash is a positional, not a flag",
			in:   []string{"-", "--json"},
			want: []string{"--json", "-"},
		},
		{
			name: "unknown flag passes through to the flag side",
			in:   []string{"pc-1", "--mystery"},
			want: []string{"--mystery", "pc-1"},
		},
		{
			name: "value flag at end of args without a value does not crash",
			in:   []string{"pc-1", "--workdir"},
			want: []string{"--workdir", "pc-1"},
		},
		{
			name: "short single-dash bool flag",
			in:   []string{"pc-1", "-json"},
			want: []string{"-json", "pc-1"},
		},
		{
			name: "multiple bool flags after positional",
			in:   []string{"pc-1", "--json", "--fail-on-orphan"},
			want: []string{"--json", "--fail-on-orphan", "pc-1"},
		},
		{
			name: "int value flag pulls its value",
			in:   []string{"pc-1", "--limit", "5"},
			want: []string{"--limit", "5", "pc-1"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fs := newReorderFlagSet()
			got := reorderFlagsBeforePositionals(tc.in, fs)
			// Normalize nil-vs-empty so reflect.DeepEqual treats them as equal.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("reorderFlagsBeforePositionals(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReorderFlagsBeforePositionalsParsesAfterReorder(t *testing.T) {
	// End-to-end shape: after reorder, flag.Parse must accept
	// previously-broken inputs like "<positional> --json" and
	// leave the positional in fs.Args().
	t.Parallel()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(discardWriter{})
	jsonFlag := fs.Bool("json", false, "")
	wdFlag := fs.String("workdir", "", "")

	in := []string{"pc-1", "--json", "--workdir", "/tmp/wd"}
	reordered := reorderFlagsBeforePositionals(in, fs)
	if err := fs.Parse(reordered); err != nil {
		t.Fatalf("flag.Parse: %v", err)
	}
	if !*jsonFlag {
		t.Errorf("expected --json to be true after reorder+parse")
	}
	if *wdFlag != "/tmp/wd" {
		t.Errorf("expected --workdir=/tmp/wd, got %q", *wdFlag)
	}
	args := fs.Args()
	if len(args) != 1 || args[0] != "pc-1" {
		t.Errorf("expected positional [pc-1], got %v", args)
	}
}

func TestIsBoolFlag(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(discardWriter{})
	fs.Bool("verbose", false, "")
	fs.String("name", "", "")

	if !isBoolFlag(fs.Lookup("verbose")) {
		t.Errorf("isBoolFlag(verbose) = false, want true")
	}
	if isBoolFlag(fs.Lookup("name")) {
		t.Errorf("isBoolFlag(name) = true, want false")
	}
}
