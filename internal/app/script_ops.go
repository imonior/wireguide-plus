package app

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/storage"
)

// This file implements the PreUp/PostUp/PreDown/PostDown script-file
// workflow and the settings backup (export/import) feature.
//
// Script files are ordinary scripts (.ps1 / .bat / .cmd / .sh) that live
// anywhere on disk — the recommended home is the app-managed scripts
// folder (sibling of tunnels/ and logs/). The .conf [Interface] hook line
// stores the full shell invocation of the chosen file (platform-appropriate
// wrapper), so the helper's existing RunScript path executes it unchanged.

// scriptHookNames is the closed set of [Interface] hook keys the editor
// may touch. Anything else is rejected before it reaches the text
// rewriter, so the frontend can't inject arbitrary keys.
var scriptHookNames = map[string]bool{
	"PreUp": true, "PostUp": true, "PreDown": true, "PostDown": true,
}

// ScriptRef describes whether a hook command references a script file
// (and where) or is a plain inline shell command.
type ScriptRef struct {
	Path   string `json:"path,omitempty"`
	IsFile bool   `json:"is_file"`
}

// maxScriptFileSize bounds Read/WriteScriptFile. Hook scripts are a few
// KiB at most; the cap keeps a buggy or hostile frontend from using the
// binding as a bulk file channel.
const maxScriptFileSize = 1 << 20

// ---------------------------------------------------------------------------
// Hook line extraction / injection (pure text — no full re-serialization)
// ---------------------------------------------------------------------------

// The rewriters below deliberately avoid config.Parse + Serialize: the
// tunnel editor shows the raw .conf text side by side with the script
// panel, and a full round-trip would silently reformat every line the
// user has hand-tuned (ordering, spacing, comments are not preserved by
// Serialize). Instead we touch only the target hook lines inside the
// [Interface] section and leave everything else byte-for-byte intact.

func hookLineRe(hook string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)^\s*` + hook + `\s*=`)
}

var sectionHeaderRe = regexp.MustCompile(`^\s*\[`)

// getHookFromText returns the effective command for a hook: single line
// verbatim, multiple lines joined with "; " for display.
func getHookFromText(content, hook string) string {
	re := hookLineRe(hook)
	var vals []string
	inInterface := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if sectionHeaderRe.MatchString(trimmed) {
			inInterface = strings.EqualFold(trimmed, "[interface]")
			continue
		}
		if inInterface && re.MatchString(line) {
			if v := strings.TrimSpace(line[strings.Index(line, "=")+1:]); v != "" {
				vals = append(vals, v)
			}
		}
	}
	return strings.Join(vals, "; ")
}

// setHookFromTextRepr rewrites the hook lines inside [Interface]. An
// empty command removes every existing line for that hook; a non-empty
// command replaces them all with a single line appended at the end of the
// section (or just before the next section header).
func setHookInText(content, hook, command string) string {
	re := hookLineRe(hook)
	eol := ""
	if strings.Contains(content, "\r\n") {
		eol = "\r"
	}
	var out []string
	inInterface := false
	inserted := false
	insert := func() {
		if inserted || command == "" {
			return
		}
		out = append(out, hook+" = "+command+eol)
		inserted = true
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if sectionHeaderRe.MatchString(trimmed) {
			// Leaving [Interface] — flush the pending line first so it
			// lands at the end of the section, right before [Peer].
			if inInterface {
				insert()
			}
			inInterface = strings.EqualFold(trimmed, "[interface]")
			out = append(out, line)
			continue
		}
		if inInterface && re.MatchString(line) {
			continue // replaced by the single new line
		}
		out = append(out, line)
	}
	if inInterface {
		insert() // [Interface] was the last section (or the only one)
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Script invocation / reference resolution
// ---------------------------------------------------------------------------

// quoteShellPath double-quotes a path for use in a shell command. Only
// embedded double quotes are escaped — cmd.exe treats backslashes inside
// quotes literally, and sh only mis-parses paths containing quote chars.
func quoteShellPath(p string) string {
	return `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
}

// scriptInvocation builds the shell command that runs a script file,
// wrapped per extension so RunScript's `cmd /C` (Windows) / `sh -c`
// (Unix) executes it correctly:
//
//	.ps1        → powershell -NoProfile -ExecutionPolicy Bypass -File "…"
//	.bat/.cmd   → "…"                   (cmd /C runs batches natively)
//	.sh         → bash "…"
//	other       → "…"                   (user knows better than we do)
func scriptInvocation(path string) string {
	q := quoteShellPath(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ps1":
		return "powershell -NoProfile -ExecutionPolicy Bypass -File " + q
	case ".sh":
		return "bash " + q
	default:
		return q
	}
}

var (
	rePSFlagFile  = regexp.MustCompile(`(?i)-file\s+"([^"]+)"`)
	reInterpFile  = regexp.MustCompile(`(?i)^(?:bash|sh|pwsh)\s+"([^"]+)"$`)
	reQuotedWhole = regexp.MustCompile(`^"([^"]+)"$`)
)

// resolveScriptRef detects whether a hook command is one of our generated
// script-file invocations. Returns the referenced path and true when the
// command references a script file, or ("", false) for inline commands.
// Heuristic on purpose: unknown shapes stay inline so hand-written
// commands are never mangled.
func resolveScriptRef(command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", false
	}
	if m := rePSFlagFile.FindStringSubmatch(command); m != nil {
		return m[1], true
	}
	if m := reInterpFile.FindStringSubmatch(command); m != nil {
		return m[1], true
	}
	if m := reQuotedWhole.FindStringSubmatch(command); m != nil {
		p := m[1]
		switch strings.ToLower(filepath.Ext(p)) {
		case ".ps1", ".bat", ".cmd", ".sh":
			return p, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Wails bindings — script files
// ---------------------------------------------------------------------------

// ScriptsDir returns (and creates) the app-managed scripts folder.
func (s *TunnelService) ScriptsDir() (string, error) {
	return storage.ScriptsDirPath()
}

// OpenScriptFile shows a native open dialog restricted to supported
// script types (any folder). Returns "" when the user cancels.
func (s *TunnelService) OpenScriptFile() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("app not initialized")
	}
	path, err := s.app.Dialog.OpenFile().
		SetTitle("Select script").
		AddFilter("PowerShell scripts", "*.ps1").
		AddFilter("Batch scripts", "*.bat").
		AddFilter("Command scripts", "*.cmd").
		AddFilter("Shell scripts", "*.sh").
		AddFilter("All files", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	return path, nil
}

// SaveScriptFile shows a native save dialog that defaults to the scripts
// folder with a preset name (tunnel + hook). The user may pick any other
// folder or name. Returns the chosen path, or "" when cancelled.
func (s *TunnelService) SaveScriptFile(defaultName string) (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("app not initialized")
	}
	scriptsDir, err := storage.ScriptsDirPath()
	if err != nil {
		return "", err
	}
	defaultName = strings.TrimSpace(defaultName)
	if defaultName == "" {
		defaultName = "script"
	}
	if filepath.Ext(defaultName) == "" {
		// Preset extension per platform so the file is runnable as-is.
		if runtime.GOOS == "windows" {
			defaultName += ".ps1"
		} else {
			defaultName += ".sh"
		}
	}
	path, err := s.app.Dialog.SaveFile().
		SetDirectory(scriptsDir).
		SetFilename(defaultName).
		AddFilter("PowerShell scripts", "*.ps1").
		AddFilter("Batch scripts", "*.bat").
		AddFilter("Command scripts", "*.cmd").
		AddFilter("Shell scripts", "*.sh").
		AddFilter("All files", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	return path, nil
}

// ReadScriptFile reads a script file for inline editing.
func (s *TunnelService) ReadScriptFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxScriptFileSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxScriptFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), nil
}

// WriteScriptFile writes script content back (or creates the file for
// "new blank"). Scripts may contain secrets echoed into logs — 0600.
func (s *TunnelService) WriteScriptFile(path, content string) error {
	if len(content) > maxScriptFileSize {
		return fmt.Errorf("content too large (%d bytes, max %d)", len(content), maxScriptFileSize)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}

// ScriptInvocation returns the shell command that runs the given script
// file (platform-appropriate wrapper). The frontend embeds the result
// into the .conf hook line.
func (s *TunnelService) ScriptInvocation(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("empty script path")
	}
	return scriptInvocation(path), nil
}

// ResolveScriptRef classifies a hook command as file-backed or inline.
func (s *TunnelService) ResolveScriptRef(command string) (*ScriptRef, error) {
	path, ok := resolveScriptRef(command)
	if !ok {
		return &ScriptRef{IsFile: false}, nil
	}
	return &ScriptRef{Path: path, IsFile: true}, nil
}

// ---------------------------------------------------------------------------
// Wails bindings — hook ↔ .conf text
// ---------------------------------------------------------------------------

// GetHookFromText extracts the current command for one hook from raw
// .conf content.
func (s *TunnelService) GetHookFromText(content, hook string) (string, error) {
	if !scriptHookNames[hook] {
		return "", fmt.Errorf("unknown hook: %s", hook)
	}
	return getHookFromText(content, hook), nil
}

// SetHookInText returns new .conf content with the hook's command set
// (empty command removes the hook lines). Only the target lines inside
// [Interface] are touched.
func (s *TunnelService) SetHookInText(content, hook, command string) (string, error) {
	if !scriptHookNames[hook] {
		return "", fmt.Errorf("unknown hook: %s", hook)
	}
	return setHookInText(content, hook, strings.TrimSpace(command)), nil
}

// ---------------------------------------------------------------------------
// Settings backup — export / import (tunnels + scripts + config.json, no logs)
// ---------------------------------------------------------------------------

// SettingsImportResult reports what one import pass produced.
type SettingsImportResult struct {
	Tunnels         []ZipImportResult `json:"tunnels"`
	Scripts         []string          `json:"scripts"`
	SettingsApplied bool              `json:"settings_applied"`
}

// ExportSettings writes tunnels + scripts + config.json (never logs) into
// a zip chosen via a native save dialog. Returns the path, or "" when the
// user cancels.
func (s *TunnelService) ExportSettings() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("app not initialized")
	}
	paths, err := storage.GetPaths()
	if err != nil {
		return "", fmt.Errorf("resolve app paths: %w", err)
	}
	dest, err := s.app.Dialog.SaveFile().
		SetDirectory(paths.ConfigDir).
		SetFilename("wireguideplus-settings-"+time.Now().Format("2006-01-02")+".zip").
		AddFilter("Zip archive", "*.zip").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if dest == "" {
		return "", nil // cancelled
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".zip") {
		dest += ".zip"
	}
	if err := s.writeSettingsZip(dest); err != nil {
		return "", err
	}
	return dest, nil
}

func (s *TunnelService) writeSettingsZip(dest string) error {
	paths, err := storage.GetPaths()
	if err != nil {
		return fmt.Errorf("resolve app paths: %w", err)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	addEntry := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	// Tunnels — read the .conf files from disk so exports are byte-exact.
	tunnelCount := 0
	if names, err := s.tunnelStore.List(); err == nil {
		for _, name := range names {
			data, err := os.ReadFile(filepath.Join(paths.TunnelsDir, name+".conf"))
			if err != nil {
				continue // vanished mid-export; skip rather than abort
			}
			if err := addEntry("tunnels/"+name+".conf", data); err != nil {
				zw.Close()
				return err
			}
			tunnelCount++
		}
	}

	// Scripts — files only, flat (no recursion: scripts are a flat folder).
	scriptCount := 0
	if entries, err := os.ReadDir(paths.ScriptsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(paths.ScriptsDir, e.Name()))
			if err != nil {
				continue
			}
			if err := addEntry("scripts/"+e.Name(), data); err != nil {
				zw.Close()
				return err
			}
			scriptCount++
		}
	}

	// App settings (config.json) — optional, may not exist yet.
	if data, err := os.ReadFile(filepath.Join(paths.ConfigDir, "config.json")); err == nil {
		if err := addEntry("config.json", data); err != nil {
			zw.Close()
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	if tunnelCount == 0 && scriptCount == 0 {
		return fmt.Errorf("nothing to export (no tunnels or scripts found)")
	}
	return nil
}

// ImportSettings asks for a settings zip (exported by ExportSettings),
// restores scripts into the scripts folder, imports tunnels, applies
// config.json, and re-points each imported tunnel's script references at
// the local scripts folder so packages stay portable across machines.
// Returns nil when the user cancels the file dialog.
func (s *TunnelService) ImportSettings() (*SettingsImportResult, error) {
	if s.app == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	path, err := s.app.Dialog.OpenFile().
		SetTitle("Import settings").
		AddFilter("Zip archive", "*.zip").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // cancelled
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()
	return s.importSettingsReader(&r.Reader)
}

func (s *TunnelService) importSettingsReader(r *zip.Reader) (*SettingsImportResult, error) {
	paths, err := storage.GetPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve app paths: %w", err)
	}
	scriptsDir, err := storage.ScriptsDirPath()
	if err != nil {
		return nil, err
	}

	result := &SettingsImportResult{Scripts: []string{}}

	// Pass 1 — scripts. Flat extraction by base name (zip-slip safe).
	for _, f := range r.File {
		rel := strings.TrimPrefix(filepath.ToSlash(f.Name), "scripts/")
		if rel == f.Name || rel == "" || strings.Contains(rel, "/") {
			continue
		}
		data, err := readZipEntry(f)
		if err != nil {
			continue
		}
		dest := filepath.Join(scriptsDir, filepath.Base(rel))
		if err := os.WriteFile(dest, data, 0600); err == nil {
			result.Scripts = append(result.Scripts, filepath.Base(rel))
		}
	}

	// Pass 2 — tunnels.
	importedNames := []string{}
	for _, f := range r.File {
		rel := strings.TrimPrefix(filepath.ToSlash(f.Name), "tunnels/")
		if rel == f.Name || rel == "" || strings.Contains(rel, "/") ||
			!strings.EqualFold(filepath.Ext(rel), ".conf") {
			continue
		}
		data, err := readZipEntry(f)
		if err != nil {
			result.Tunnels = append(result.Tunnels, ZipImportResult{Name: filepath.Base(rel), Error: err.Error()})
			continue
		}
		base := strings.TrimSuffix(filepath.Base(rel), ".conf")
		name := s.zipUniqueName(base)
		if _, err := s.ImportConfig(name, string(data)); err != nil {
			result.Tunnels = append(result.Tunnels, ZipImportResult{Name: base, Error: err.Error()})
		} else {
			importedNames = append(importedNames, name)
			result.Tunnels = append(result.Tunnels, ZipImportResult{Name: name})
		}
	}

	// Pass 3 — app settings.
	for _, f := range r.File {
		if filepath.ToSlash(f.Name) != "config.json" {
			continue
		}
		data, err := readZipEntry(f)
		if err != nil {
			break
		}
		cfgPath := filepath.Join(paths.ConfigDir, "config.json")
		if err := os.WriteFile(cfgPath, data, 0600); err == nil {
			result.SettingsApplied = true
		}
		break
	}

	// Pass 4 — re-point script references at the local scripts folder so
	// a package exported on machine A resolves on machine B.
	if len(result.Scripts) > 0 {
		local := map[string]bool{}
		for _, n := range result.Scripts {
			local[strings.ToLower(n)] = true
		}
		for _, name := range importedNames {
			cfg, err := s.tunnelStore.Load(name)
			if err != nil {
				continue
			}
			changed := false
			hooks := []*string{
				&cfg.Interface.PreUp, &cfg.Interface.PostUp,
				&cfg.Interface.PreDown, &cfg.Interface.PostDown,
			}
			for _, h := range hooks {
				if *h == "" {
					continue
				}
				ref, ok := resolveScriptRef(*h)
				if !ok {
					continue
				}
				if !local[strings.ToLower(filepath.Base(ref))] {
					continue
				}
				*h = scriptInvocation(filepath.Join(scriptsDir, filepath.Base(ref)))
				changed = true
			}
			if changed {
				if err := s.tunnelStore.Save(cfg); err != nil {
					continue
				}
			}
		}
	}

	if len(result.Tunnels) == 0 && len(result.Scripts) == 0 && !result.SettingsApplied {
		return nil, fmt.Errorf("no tunnels, scripts or config.json found in zip")
	}
	return result, nil
}

// readZipEntry reads one entry with the shared zip-bomb size cap.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxZipEntrySize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxZipEntrySize {
		return nil, fmt.Errorf("entry exceeds %d bytes", maxZipEntrySize)
	}
	return data, nil
}
