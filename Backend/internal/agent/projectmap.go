package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// agentRulesMaxBytes caps the project rules injected into every prompt. A large
// AGENTS.md or CAL.md is still useful, but not at the cost of the conversation.
const agentRulesMaxBytes = 16 << 10

// ProjectMap is the scanned summary of a workspace, injected into the system prompt.
type ProjectMap struct {
	OS        string
	Shell     string
	Workspace string
	Projects  []ProjectInfo
	Rules     string
}

// ProjectInfo describes one project found in the workspace.
type ProjectInfo struct {
	Name        string
	Root        string // relative to workspace
	Type        string // "node", "go"
	PackageMgr  string // "pnpm", "npm", "go"
	ConfigFiles []string
	Scripts     []string
}

// ScanProjectMap walks the workspace root (depth 2) and builds a ProjectMap.
// It is called fresh on every run; no cache invalidation. The scan is bounded
// and stops when ctx is cancelled, so a cancelled turn does not keep walking a
// large workspace.
func ScanProjectMap(ctx context.Context, workspaceRoot string) ProjectMap {
	pm := ProjectMap{
		OS:        runtime.GOOS,
		Shell:     "sh",
		Workspace: workspaceRoot,
		Projects:  nil,
		Rules:     "",
	}
	if runtime.GOOS == "windows" {
		pm.Shell = "PowerShell"
	}

	// Load AGENTS.md / CAL.md if present
	if rules := loadAgentRules(ctx, workspaceRoot); rules != "" {
		pm.Rules = rules
	}

	if ctx.Err() != nil {
		return pm
	}

	// Walk depth 2 from workspace root
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return pm
	}

	seen := make(map[string]bool)

	for _, entry := range entries {
		if ctx.Err() != nil {
			return pm
		}
		if !entry.IsDir() {
			continue
		}
		// Skip hidden and known noise
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "build" || name == "dist" || name == ".git" {
			continue
		}

		projectDir := filepath.Join(workspaceRoot, name)
		info := detectProject(projectDir, name)
		if info != nil {
			key := info.Root
			if !seen[key] {
				pm.Projects = append(pm.Projects, *info)
				seen[key] = true
			}
		}

		// Also check one level deeper (depth 2)
		subEntries, _ := os.ReadDir(projectDir)
		for _, sub := range subEntries {
			if ctx.Err() != nil {
				return pm
			}
			if !sub.IsDir() {
				continue
			}
			subName := sub.Name()
			if strings.HasPrefix(subName, ".") || subName == "node_modules" || subName == "build" || subName == "dist" || subName == ".git" {
				continue
			}
			subDir := filepath.Join(projectDir, subName)
			subInfo := detectProject(subDir, filepath.Join(name, subName))
			if subInfo != nil {
				key := subInfo.Root
				if !seen[key] {
					pm.Projects = append(pm.Projects, *subInfo)
					seen[key] = true
				}
			}
		}
	}

	// Stable sort for prompt caching
	sort.Slice(pm.Projects, func(i, j int) bool {
		return pm.Projects[i].Root < pm.Projects[j].Root
	})

	return pm
}

// detectProject checks a directory for known project markers.
func detectProject(dir, relPath string) *ProjectInfo {
	var info ProjectInfo
	info.Root = relPath
	info.Name = filepath.Base(dir)

	// Check for package.json (Node project)
	if pkgPath := filepath.Join(dir, "package.json"); fileExists(pkgPath) {
		info.Type = "node"
		info.ConfigFiles = append(info.ConfigFiles, "package.json")
		if scripts := extractScripts(pkgPath); len(scripts) > 0 {
			info.Scripts = scripts
		}
		// Detect package manager from lockfile
		if fileExists(filepath.Join(dir, "pnpm-lock.yaml")) {
			info.PackageMgr = "pnpm"
			info.ConfigFiles = append(info.ConfigFiles, "pnpm-lock.yaml")
		} else if fileExists(filepath.Join(dir, "package-lock.json")) {
			info.PackageMgr = "npm"
			info.ConfigFiles = append(info.ConfigFiles, "package-lock.json")
		} else if fileExists(filepath.Join(dir, "yarn.lock")) {
			info.PackageMgr = "yarn"
			info.ConfigFiles = append(info.ConfigFiles, "yarn.lock")
		} else {
			info.PackageMgr = "npm"
		}
		// Check for components.json (shadcn/ui)
		if fileExists(filepath.Join(dir, "components.json")) {
			info.ConfigFiles = append(info.ConfigFiles, "components.json")
		}
	}

	// Check for go.mod (Go project)
	if goModPath := filepath.Join(dir, "go.mod"); fileExists(goModPath) {
		info.Type = "go"
		info.PackageMgr = "go"
		info.ConfigFiles = append(info.ConfigFiles, "go.mod")
	}

	// If neither, not a recognized project
	if info.Type == "" {
		return nil
	}

	return &info
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func extractScripts(pkgPath string) []string {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}
	// Simple JSON parsing for "scripts" object
	content := string(data)
	scriptsStart := strings.Index(content, `"scripts"`)
	if scriptsStart == -1 {
		return nil
	}
	braceStart := strings.Index(content[scriptsStart:], `{`)
	if braceStart == -1 {
		return nil
	}
	braceStart += scriptsStart
	braceEnd := findMatchingBrace(content, braceStart)
	if braceEnd == -1 {
		return nil
	}
	scriptsJSON := content[braceStart : braceEnd+1]
	var scripts map[string]string
	if err := jsonUnmarshal([]byte(scriptsJSON), &scripts); err != nil {
		return nil
	}
	keys := make([]string, 0, len(scripts))
	for k := range scripts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func findMatchingBrace(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// loadAgentRules reads AGENTS.md or CAL.md from workspace root, bounded so a
// large rules file cannot dominate the prompt. It stops reading on cancellation.
func loadAgentRules(ctx context.Context, root string) string {
	for _, name := range []string{"AGENTS.md", "CAL.md"} {
		if ctx.Err() != nil {
			return ""
		}
		path := filepath.Join(root, name)
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(file, agentRulesMaxBytes+1))
		_ = file.Close()
		if readErr != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if len(data) > agentRulesMaxBytes {
			text = strings.TrimSpace(string(data[:agentRulesMaxBytes])) +
				"\n… [project rules truncated at 16 KB]"
		}
		return text
	}
	return ""
}

// ProjectMapString formats the ProjectMap for injection into the system prompt.
func (pm ProjectMap) ProjectMapString() string {
	if len(pm.Projects) == 0 && pm.Rules == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("=== ENVIRONMENT ===\n")
	b.WriteString("OS: " + pm.OS + "\n")
	b.WriteString("Shell: " + pm.Shell + "\n")
	b.WriteString("Workspace: " + pm.Workspace + "\n\n")

	if len(pm.Projects) > 0 {
		b.WriteString("Projects:\n")
		for _, p := range pm.Projects {
			b.WriteString("  - " + p.Root + "/")
			if p.Type != "" {
				b.WriteString(" (" + p.Type)
				if p.PackageMgr != "" {
					b.WriteString(", " + p.PackageMgr)
				}
				b.WriteString(")")
			}
			if len(p.ConfigFiles) > 0 {
				b.WriteString(" [" + strings.Join(p.ConfigFiles, ", ") + "]")
			}
			if len(p.Scripts) > 0 {
				b.WriteString(" scripts: " + strings.Join(p.Scripts, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if pm.Rules != "" {
		b.WriteString("=== USER RULES (from AGENTS.md/CAL.md) ===\n")
		b.WriteString(pm.Rules + "\n\n")
	}

	b.WriteString("Rule: Run commands with cwd set to the project folder that owns the config file.\n")

	return b.String()
}
