package gitread

import (
	"fmt"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

type repo struct {
	t    *testing.T
	git  string
	dir  string
	env  []string
	tick int
}

func newRepo(t *testing.T, dir string) *repo {
	r := testutil.NewGitRepo(t, dir)
	return &repo{t: r.T, git: r.Git, dir: r.Dir, env: r.Env, tick: r.Tick}
}

func (r *repo) asShared() *testutil.GitRepo {
	return &testutil.GitRepo{T: r.t, Git: r.git, Dir: r.dir, Env: r.env, Tick: r.tick}
}

func (r *repo) run(args ...string) string {
	g := r.asShared()
	out := g.Run(args...)
	r.tick = g.Tick
	return out
}

func (r *repo) write(name, body string) { r.asShared().Write(name, body) }
func (r *repo) commit(message string) {
	g := r.asShared()
	g.Commit(message)
	r.tick = g.Tick
}

func script(t *testing.T, dir, name, body string) string {
	return testutil.WriteScript(t, dir, name, body)
}

type planted struct {
	*repo
	inner *testutil.PlantedRepo
	root  string
	lazy  string
	log   string
}

func plantRepo(t *testing.T) *planted {
	p := testutil.PlantGitRepo(t)
	return &planted{
		repo:  &repo{t: p.T, git: p.Git, dir: p.Dir, env: p.Env, tick: p.Tick},
		inner: p,
		root:  p.Root,
		lazy:  p.Lazy,
		log:   p.Log,
	}
}

func (p *planted) ran() string { return p.inner.Ran() }

func snapshot(t *testing.T, roots ...string) map[string]string {
	return testutil.SnapshotTree(t, roots...)
}

// lines is n numbered lines; every second one starts with prefix.
func lines(n int, prefix string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			b.WriteString(prefix)
		}
		fmt.Fprintf(&b, "line %d %s\n", i, strings.Repeat("x", i%7))
	}
	return b.String()
}
