// The kind-coverage guard, in Go (cross-os-tcd).
//
// The vitest suite already proves that every kind a page DECLARES has a
// renderer. It cannot prove the other direction, and the other direction is
// the one that bit: ruleBuilder, keymapEditor and conflictResolver were all
// registered, all unit-tested, and reachable from no daemon page at all — the
// flagship "make it yours" flow was dead code behind a green suite. The e2e
// fixture declared the three on its own copy of a page, which is exactly how a
// gap like that stays invisible.
//
// Declared ⊆ Registered is a real gate. Registered ⊆ Declared is not, because a
// registry is allowed to hold a kind before any page asks for it, and the
// grace-period aliases are exactly that on purpose. So the second direction
// holds against declared PLUS alias, and the aliases have to be readable as
// aliases rather than inferred from a comment.

package shell

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The registry file, relative to this package. A Go-only checkout will not have
// it, and the tests below skip rather than fail in that case — the same shape
// the bindings tests use, because a rule that only works with a JavaScript
// toolchain installed is a rule a contributor can skip by not installing it.
const registryFile = "../frontend/src/controls/index.tsx"

var (
	mapLiteral   = regexp.MustCompile(`(?s)const RENDERERS = new Map<string, ControlRenderer>\(\[(.*?)\n\]\)`)
	aliasLiteral = regexp.MustCompile(`(?s)RENDERER_ALIASES: Record<string, ControlRenderer> = \{(.*?)\n\}`)
	mapEntry     = regexp.MustCompile(`\[\s*'([A-Za-z]\w*)'\s*,`)
	aliasField   = regexp.MustCompile(`(?m)^\s*([A-Za-z]\w*):`)
	kindField    = regexp.MustCompile(`"kind":\s*"([A-Za-z]\w*)"`)
)

// registry reads the kind keys out of the two named structures in the registry
// file. The file is the source of truth and this reads it rather than a copy,
// because a copy is a second list to forget — which is how "Service.ApplyProfile
// is bound but absent from the frozen list" happened.
func registry(t *testing.T) (kinds, aliases []string) {
	t.Helper()
	raw, err := os.ReadFile(registryFile)
	if err != nil {
		t.Skipf("the shell registry is not readable at %s (%v) — a Go-only checkout; renderers.test.tsx in app/frontend covers the same file when the frontend is present", registryFile, err)
	}
	src := string(raw)

	body := mapLiteral.FindStringSubmatch(src)
	if body == nil {
		t.Fatalf("no RENDERERS map literal found in %s — the registry's shape changed and this guard has to follow it", registryFile)
	}
	alias := aliasLiteral.FindStringSubmatch(src)
	if alias == nil {
		t.Fatalf("no RENDERER_ALIASES literal found in %s — the aliases are no longer readable as aliases, so this guard cannot tell an alias from a dead kind", registryFile)
	}
	for _, m := range mapEntry.FindAllStringSubmatch(body[1], -1) {
		kinds = append(kinds, m[1])
	}
	for _, m := range aliasField.FindAllStringSubmatch(alias[1], -1) {
		aliases = append(aliases, m[1])
	}
	if len(kinds) == 0 {
		t.Fatalf("read zero kinds out of the registry in %s", registryFile)
	}
	return kinds, aliases
}

// declaredKinds walks the Go contributions and reads the `kind` field out of
// every UIContribution schema. A contribution is a .go file that mentions
// UIContribution and is not a test — the same signal the frontend guard uses, so
// the two cannot drift onto different sets.
func declaredKinds(t *testing.T) map[string][]string {
	t.Helper()
	seen := map[string][]string{}
	roots := []string{".", filepath.Join("..", "..", "plugins"), filepath.Join("..", "..", "extensions")}
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil || !strings.Contains(string(raw), "UIContribution") {
				return nil
			}
			for _, m := range kindField.FindAllStringSubmatch(string(raw), -1) {
				seen[m[1]] = append(seen[m[1]], filepath.ToSlash(path))
			}
			return nil
		})
	}
	return seen
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestEveryDeclaredKindHasARenderer is the direction the vitest suite already
// covers, repeated here so the gate does not need npm to run.
func TestEveryDeclaredKindHasARenderer(t *testing.T) {
	kinds, aliases := registry(t)
	have := map[string]bool{}
	for _, k := range append(append([]string{}, kinds...), aliases...) {
		have[k] = true
	}
	declared := declaredKinds(t)
	for _, kind := range sorted(mapKeys(declared)) {
		if !have[kind] {
			where := declared[kind]
			sort.Strings(where)
			t.Errorf("a page declares kind %q (%s) and the shell registry has no renderer for it — the page would land in UnsupportedControl", kind, where[0])
		}
	}
}

// assembledKinds are the kinds no page declares because a CONTROL builds the
// control object itself — the extension list assembling the detail card for the
// row a person opened, rather than a page declaring a detail control that is
// empty until something is selected.
//
// They are named here with the place that builds them, because "no page
// declares it" is not the same as "nothing reaches it" and a guard that cannot
// tell the two is a guard that has to be switched off.
var assembledKinds = map[string]string{
	"pluginDetail": "PluginListControl.tsx assembles it for the row a person opened",
}

// TestEveryRegisteredKindIsReachable is the direction nothing covered. A
// registered kind that no page declares, that is not a named alias, and that no
// control assembles is dead weight in the registry: a renderer nothing can
// reach is one nobody will ever test against a real page.
func TestEveryRegisteredKindIsReachable(t *testing.T) {
	kinds, aliases := registry(t)
	isAlias := map[string]bool{}
	for _, k := range aliases {
		isAlias[k] = true
	}
	declared := declaredKinds(t)
	var dead []string
	for _, kind := range kinds {
		if declared[kind] == nil && !isAlias[kind] && assembledKinds[kind] == "" {
			dead = append(dead, kind)
		}
	}
	for _, kind := range dead {
		t.Errorf("kind %q is registered and nothing reaches it: no page declares it, it is not in RENDERER_ALIASES, and no control assembles it. Either a page should, it is a retired spelling, or the renderer is dead weight", kind)
	}
}

// TestEveryAssembledKindIsStillBuilt stops that list becoming the same escape
// hatch the guard exists to close. A kind kept here after the control stopped
// assembling it is a kind nothing reaches, with a note explaining why.
func TestEveryAssembledKindIsStillBuilt(t *testing.T) {
	kinds, _ := registry(t)
	have := map[string]bool{}
	for _, k := range kinds {
		have[k] = true
	}
	for kind, builder := range assembledKinds {
		if !have[kind] {
			t.Errorf("assembled kind %q is no longer registered — %s, so remove it from assembledKinds rather than leaving a name for a kind that cannot be drawn", kind, builder)
			continue
		}
		src, err := os.ReadFile(filepath.Join("..", "frontend", "src", "controls", "PluginListControl.tsx"))
		if err != nil {
			t.Fatalf("reading the assembling control: %v", err)
		}
		if !strings.Contains(string(src), "kind: '"+kind+"'") {
			t.Errorf("assembled kind %q is listed as built by %s, but that file no longer builds it — remove it from assembledKinds or the guard is guarding a name", kind, builder)
		}
	}
}

// TestNoAliasIsStillDeclared is what keeps the grace period finite. An alias a
// page still uses is not a grace period any more — it is the current spelling,
// and leaving it in the alias list means the retirement is never finished and
// nobody can tell the two apart by reading the registry.
func TestNoAliasIsStillDeclared(t *testing.T) {
	_, aliases := registry(t)
	declared := declaredKinds(t)
	for _, alias := range aliases {
		if where := declared[alias]; where != nil {
			sort.Strings(where)
			t.Errorf("alias %q is still declared by %s. An alias a page uses is the current spelling, not a retired one — move it out of RENDERER_ALIASES into the map so the retirement actually completes", alias, where[0])
		}
	}
}

func mapKeys(m map[string][]string) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}
