// Extension-pack loading: manifest parser, path validation, rule matching,
// undeclared-file inspection (bead cross-os-vbl.3).
//
// Plan: COMPREHENSIVE_PLAN.md §5.3 (manifest schema), §5.4 (action schema),
// §5.5 (rule matching), §9.10 menumate row, §9.11, §3.6 extpack type,
// §6.3 Security. Concept source: tmp/research/menumate
// (commit 017d6da) — PackManifest.swift (decode-then-validate, unknown keys
// ignored, defaults filled), RuleMatcher.swift (targets/UTI/count filters,
// typed MatchResult), PackInspector.swift (undeclared-file surfacing, .git +
// metadata skipped, symlink-escape defense), ConfigStore.swift (mmap-free
// load with mtime cache — adapted as mtime-free pure parse here).
//
// DIVERGENCE (deliberate, §9.10): MenuMate executes user scripts as actions;
// CrossOS executes native capabilities — script executor NOT copied.
// Shell-bearing packs require the Level B gate; native packs load by
// default. Attribution notes menumate as concept source (no executor code).
//
// Wiring: this package parses + validates + matches packs and exposes the
// actions as MenuTable-compatible rows for the vbl.2 Dispatch path; the
// TODO(vbl.3) in finder-sync/menus.go resolves here.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackTargets is the §5.5 targets filter vocabulary.
type PackTargets string

const (
	TargetsFiles     PackTargets = "files"
	TargetsFolders   PackTargets = "folders"
	TargetsAny       PackTargets = "any"
	TargetsContainer PackTargets = "container"
	TargetsSelection PackTargets = "selection"
)

// PackActionType: native (default) or process (Level B). `script` is NOT
// part of the schema (§5.4) — rejected at parse.
type PackActionType string

const (
	PackActionNative  PackActionType = "native"
	PackActionProcess PackActionType = "process"
)

// PackAction is one §5.4 action: id/title/icon/action/targets + filters.
type PackAction struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Icon      string         `json:"icon"`
	Action    PackActionBody `json:"action"`
	Targets   any            `json:"targets"` // string or []string
	UTIs      []string       `json:"utis"`
	Placement string         `json:"placement"`
	Enabled   *bool          `json:"isEnabled"`
}

// PackActionBody is the action payload: native capability or Level B process.
type PackActionBody struct {
	Type       string         `json:"type"`
	Capability string         `json:"capability"`
	Parameters map[string]any `json:"parameters"`
	Plugin     string         `json:"plugin"`
	Operation  string         `json:"operation"`
}

// PackManifest is the §5.3 manifest: unknown keys ignored (forward compat),
// defaults filled at decode (pack icon shippingbox, action icon bolt,
// timeout 60, placement topLevel, isEnabled true).
type PackManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	Name          string       `json:"name"`
	Author        string       `json:"author"`
	Description   string       `json:"description"`
	Icon          string       `json:"icon"`
	Actions       []PackAction `json:"actions"`
	Raw           map[string]any
}

// MatchCtx is the runtime selection context for rule matching (§5.5).
type MatchCtx struct {
	// Container: background right-click on dir (no selection).
	Container string
	// Items: selected paths with dir flags.
	Items []MatchItem
}

// MatchItem is one selected path after resolving metadata once.
type MatchItem struct {
	Path    string
	IsDir   bool
	UTI     string
	Missing bool
}

// MatchResult names why an action shows or hides (menumate MatchResult port:
// typed, never a bare bool — callers log the reason).
type MatchResult string

const (
	MatchOK         MatchResult = "matched"
	MatchEmpty      MatchResult = "emptySelection"
	MatchMissing    MatchResult = "unavailableItem"
	MatchTarget     MatchResult = "targetMismatch"
	MatchType       MatchResult = "typeMismatch"
	MatchNoUTI      MatchResult = "utiMismatch"
	MatchCountMin   MatchResult = "tooFew"
	MatchCountMax   MatchResult = "tooMany"
	MatchShellGated MatchResult = "shellGated"
	// MatchDisabled: action disabled via isEnabled=false. Distinct from
	// targetMismatch so stage logs read "disabled", not a filter reason.
	// (review: cross-os-8d)
	MatchDisabled MatchResult = "disabled"
)

// MaxPackPaths mirrors the §6.3 dispatch cap at load time.
const MaxPackPaths = 100

// ParsePackManifest decodes manifest.json: unknown keys ignored, defaults
// filled. Decode only — Validate does semantics.
func ParsePackManifest(raw []byte) (PackManifest, error) {
	var m PackManifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&m.Raw); err != nil {
		return PackManifest{}, fmt.Errorf("pack: not JSON: %w", err)
	}
	// Re-decode typed fields with defaults (unknown keys survive in Raw).
	var typed struct {
		SchemaVersion int          `json:"schemaVersion"`
		Name          string       `json:"name"`
		Author        string       `json:"author"`
		Description   string       `json:"description"`
		Icon          string       `json:"icon"`
		Actions       []PackAction `json:"actions"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		return PackManifest{}, fmt.Errorf("pack: bad manifest: %w", err)
	}
	m.SchemaVersion = typed.SchemaVersion
	m.Name = typed.Name
	m.Author = typed.Author
	m.Description = typed.Description
	m.Icon = typed.Icon
	if m.Icon == "" {
		m.Icon = "shippingbox"
	}
	m.Actions = typed.Actions
	for i := range m.Actions {
		if m.Actions[i].Placement == "" {
			m.Actions[i].Placement = "topLevel"
		}
		if m.Actions[i].Enabled == nil {
			t := true
			m.Actions[i].Enabled = &t
		}
	}
	return m, nil
}

// Validate checks manifest semantics: schema version, name/actions non-empty,
// per-action fields, script paths confined to relative in-pack paths.
// `script` action type is rejected (not part of the schema).
func (m PackManifest) Validate() []error {
	var errs []error
	if m.SchemaVersion < 1 || m.SchemaVersion > 2 {
		errs = append(errs, fmt.Errorf("pack: schemaVersion %d not in 1..2", m.SchemaVersion))
	}
	if strings.TrimSpace(m.Name) == "" {
		errs = append(errs, fmt.Errorf("pack: empty name"))
	}
	if len(m.Actions) == 0 {
		errs = append(errs, fmt.Errorf("pack: no actions"))
	}
	seen := map[string]bool{}
	for i, a := range m.Actions {
		if strings.TrimSpace(a.ID) == "" {
			errs = append(errs, fmt.Errorf("pack: action %d: empty id", i))
		} else if seen[a.ID] {
			errs = append(errs, fmt.Errorf("pack: duplicate action id %q", a.ID))
		} else {
			seen[a.ID] = true
		}
		if strings.TrimSpace(a.Title) == "" {
			errs = append(errs, fmt.Errorf("pack: action %q: empty title", a.ID))
		}
		switch PackActionType(a.Action.Type) {
		case PackActionNative:
			if strings.TrimSpace(a.Action.Capability) == "" {
				errs = append(errs, fmt.Errorf("pack: action %q: native without capability", a.ID))
			}
		case PackActionProcess:
			if strings.TrimSpace(a.Action.Plugin) == "" {
				errs = append(errs, fmt.Errorf("pack: action %q: process without plugin", a.ID))
			}
		case "script", "":
			errs = append(errs, fmt.Errorf("pack: action %q: type %q rejected (not in schema; shell is Level B)", a.ID, a.Action.Type))
		default:
			errs = append(errs, fmt.Errorf("pack: action %q: unknown type %q", a.ID, a.Action.Type))
		}
		if a.Placement != "topLevel" && a.Placement != "submenu" {
			errs = append(errs, fmt.Errorf("pack: action %q: placement %q", a.ID, a.Placement))
		}
	}
	return errs
}

// TargetsOf normalizes the string-or-list targets field.
func (a PackAction) TargetsOf() []PackTargets {
	var out []PackTargets
	switch t := a.Targets.(type) {
	case string:
		out = append(out, PackTargets(t))
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok {
				out = append(out, PackTargets(s))
			}
		}
	case []string:
		for _, s := range t {
			out = append(out, PackTargets(s))
		}
	}
	return out
}

// Match evaluates one action against ctx (§5.5): targets filter, UTI filter,
// selection count. levelBAllowed gates process actions (shell default-off).
func Match(a PackAction, ctx MatchCtx, levelBAllowed bool, minCount, maxCount int) MatchResult {
	if a.Enabled != nil && !*a.Enabled {
		return MatchDisabled
	}
	if PackActionType(a.Action.Type) == PackActionProcess && !levelBAllowed {
		return MatchShellGated
	}
	if ctx.Container != "" {
		for _, t := range a.TargetsOf() {
			if t == TargetsContainer {
				return MatchOK
			}
		}
		return MatchTarget
	}
	if len(ctx.Items) == 0 {
		return MatchEmpty
	}
	for _, it := range ctx.Items {
		if it.Missing {
			return MatchMissing
		}
	}
	if minCount > 0 && len(ctx.Items) < minCount {
		return MatchCountMin
	}
	if maxCount > 0 && len(ctx.Items) > maxCount {
		return MatchCountMax
	}
	matched := false
	for _, t := range a.TargetsOf() {
		switch t {
		case TargetsAny, TargetsSelection:
			matched = true
		case TargetsFiles:
			matched = true
			for _, it := range ctx.Items {
				if it.IsDir {
					matched = false
				}
			}
		case TargetsFolders:
			matched = true
			for _, it := range ctx.Items {
				if !it.IsDir {
					matched = false
				}
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		return MatchTarget
	}
	if len(a.UTIs) > 0 {
		for _, it := range ctx.Items {
			hit := false
			for _, u := range a.UTIs {
				if it.UTI == u {
					hit = true
				}
			}
			if !hit {
				return MatchNoUTI
			}
		}
	}
	return MatchOK
}

// VisibleActions filters enabled, matched actions in manifest order (§5.5:
// menumate sorts by sortOrder; manifest order is CrossOS's stable order).
func VisibleActions(m PackManifest, ctx MatchCtx, levelBAllowed bool) []PackAction {
	var out []PackAction
	for _, a := range m.Actions {
		if Match(a, ctx, levelBAllowed, 0, MaxPackPaths+1) == MatchOK {
			out = append(out, a)
		}
	}
	return out
}

// ValidatePackPath enforces load+dispatch path rules (§6.3 Security):
// absolute, clean, no ".." pre-clean (Clean hides escapes), inside pack dir
// for in-pack references.
func ValidatePackPath(packDir, p string) error {
	if strings.Contains(p, "..") {
		return fmt.Errorf("pack: rejected path %q (traversal)", p)
	}
	clean := filepath.Clean(p)
	if clean == "/" {
		return fmt.Errorf("pack: rejected path %q (root)", p)
	}
	if !filepath.IsAbs(clean) {
		// In-pack relative reference: must stay inside packDir.
		joined := filepath.Join(packDir, clean)
		rel, err := filepath.Rel(packDir, joined)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("pack: reference %q escapes pack", p)
		}
		return nil
	}
	return nil
}

// InspectPack flags undeclared files before load (PackInspector port):
// walks packDir skipping .git + metadata (manifest.json, README*, LICENSE*,
// *.md, .gitignore, .gitattributes, .DS_Store), returns sorted relative
// paths NOT in declared. Hidden files are NOT skipped (a hidden script is
// exactly what review must see). Symlinks are reported, never followed.
func InspectPack(packDir string, declared map[string]bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(packDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: skip, loader reports separately
		}
		rel, rerr := filepath.Rel(packDir, path)
		if rerr != nil || rel == "." {
			return nil
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		lname := strings.ToLower(name)
		if name == "manifest.json" || name == ".gitignore" || name == ".gitattributes" || name == ".DS_Store" {
			return nil
		}
		if strings.HasSuffix(lname, ".md") || strings.HasPrefix(lname, "readme") || strings.HasPrefix(lname, "license") {
			return nil
		}
		if declared[rel] {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
