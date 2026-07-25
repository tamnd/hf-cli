package cli

import (
	"context"
	"io"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/hf-cli/hf"
)

// files.go holds the two commands that emit bytes rather than records. They are
// the reason the tool is useful for actual file work and not only metadata, and
// they are escape hatches precisely because a file is not a record: piping a
// safetensors shard through a JSON renderer would be a mistake in every
// direction.

// clientFrom reaches the one client kit built for this run. Escape-hatch
// commands do not get the kit:"inject" treatment, so they ask for it here, and
// asking here means they share the run's pacing and cache with every operation.
func clientFrom(ctx context.Context) (*hf.Client, error) {
	st := kit.FromContext(ctx)
	if st == nil {
		return nil, errs.New(errs.KindGeneric, "no run state on the context")
	}
	v, err := st.Client(ctx)
	if err != nil {
		return nil, err
	}
	c, ok := v.(*hf.Client)
	if !ok {
		return nil, errs.New(errs.KindGeneric, "the run has no hf client")
	}
	return c, nil
}

type catCmd struct {
	rev     string
	pointer bool
}

func newCatCmd() kit.Command {
	c := &catCmd{}
	return kit.Command{
		Use:   "cat <repo> <path>",
		Short: "Write one file from a repository to stdout",
		Long: "cat streams the bytes straight through, so a multi-gigabyte shard costs no\n" +
			"memory and is never written to the cache. LFS files resolve to their payload;\n" +
			"pass --pointer for the git pointer text instead.",
		Group: "repo",
		Args:  kit.ExactArgs(2),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *catCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.rev, "rev", "", "branch, tag, or commit sha")
	f.BoolVar(&c.pointer, "pointer", false, "return the LFS pointer text rather than the payload")
}

func (c *catCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	kind, id, err := hf.ResolveRepo(args[0])
	if err != nil {
		return err
	}
	_, err = cl.Download(ctx, kind, id, c.rev, args[1], c.pointer, os.Stdout)
	return err
}

type readmeCmd struct {
	rev  string
	body bool
}

func newReadmeCmd() kit.Command {
	c := &readmeCmd{}
	return kit.Command{
		Use:   "readme <repo>",
		Short: "Write a repository's card to stdout",
		Long: "readme prints the whole README including its front matter. Use --body to drop\n" +
			"the front matter, or `hf card` to read the front matter as a record.",
		Group: "repo",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *readmeCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.rev, "rev", "", "branch, tag, or commit sha")
	f.BoolVar(&c.body, "body", false, "print the prose only, without the front matter")
}

func (c *readmeCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	kind, id, err := hf.ResolveRepo(args[0])
	if err != nil {
		return err
	}
	text, err := cl.Readme(ctx, kind, id, c.rev)
	if err != nil {
		return err
	}
	if c.body {
		_, body, err := hf.ParseReadme(text)
		if err != nil {
			return err
		}
		text = body
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}
