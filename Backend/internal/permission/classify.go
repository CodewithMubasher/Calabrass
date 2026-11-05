package permission

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Action is one thing a tool is about to do, described well enough to decide
// how much trust it needs and to show to the person who is asked.
type Action struct {
	Tool    string
	Tier    Tier
	Summary string // one line: "Run: npm install"
	Detail  string // optional longer text, for example the full command
	// Pattern is what "allow this kind of command in this workspace" remembers.
	// Empty when the action must be asked every time.
	Pattern string
}

// Classify works out the action a tool call stands for. workspace is the
// project root, used to tell a path inside it from one outside it.
func Classify(tool string, input json.RawMessage, workspace string) Action {
	switch tool {
	case "read_file", "list_dir", "glob", "grep", "read_skill",
		"ask_questions", "delegate_task", "report_task", "delegate":
		return Action{Tool: tool, Tier: TierObserve, Summary: tool}

	case "create_dir", "write_file", "edit_file", "edit_lines":
		act := Action{Tool: tool, Tier: TierChange, Summary: describeFileAction(tool, input)}
		// Some files are not data: something else runs them (git, an editor, a
		// CI service, the shell at login). Changing one is running code later,
		// so it is asked about like running a command, every time.
		if path := pathArg(input); runsCode(path) {
			act.Tier = TierExecute
			act.Summary = "Change a file that runs code: " + clip(path, 120)
		}
		return act

	case "run_command":
		var args struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
		}
		_ = json.Unmarshal(input, &args)
		return ClassifyCommand(args.Command, args.Cwd, workspace)

	case "schedule_task":
		var args struct {
			Prompt      string `json:"prompt"`
			Instruction string `json:"instruction"`
		}
		_ = json.Unmarshal(input, &args)
		text := args.Prompt
		if text == "" {
			text = args.Instruction
		}
		// A booked task runs later with nobody watching, so booking one is an
		// act of execution in its own right. Its own scope is set by the
		// person, never by the model.
		return Action{
			Tool: tool, Tier: TierExecute,
			Summary: "Schedule a task: " + clip(text, 120),
			Detail:  text,
			Pattern: "schedule_task",
		}
	}

	return classifyConnectorTool(tool, input)
}

func pathArg(input json.RawMessage) string {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(input, &args)
	return args.Path
}

func describeFileAction(tool string, input json.RawMessage) string {
	path := pathArg(input)
	if path == "" {
		return tool
	}
	return tool + ": " + path
}

// codeFileNames are files that a program other than the editor reads and
// acts on: shell start-up files, git configuration, package-manager settings.
var codeFileNames = map[string]bool{
	".envrc": true, ".gitmodules": true, ".gitattributes": true, ".gitconfig": true,
	".npmrc": true, ".yarnrc": true, ".yarnrc.yml": true, ".pypirc": true,
	".pre-commit-config.yaml": true, ".gitlab-ci.yml": true,
	".bashrc": true, ".bash_profile": true, ".zshrc": true, ".profile": true,
}

// codeDirNames are directories whose contents are run by tools or services.
var codeDirNames = map[string]bool{
	".husky": true, ".devcontainer": true, ".ssh": true,
}

// runsCode reports whether a path names a file that something else will run.
// It reads the path as written, lexically, and errs towards asking.
func runsCode(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	for i, part := range parts {
		trimmed := strings.TrimRight(part, ". ")
		switch {
		case trimmed == ".git" || strings.HasPrefix(trimmed, ".git:") || strings.HasPrefix(trimmed, "git~"):
			return true
		case codeDirNames[trimmed]:
			return true
		case i == len(parts)-1 && codeFileNames[trimmed]:
			return true
		case trimmed == ".github" && i+1 < len(parts) && strings.TrimRight(parts[i+1], ". ") == "workflows":
			return true
		case trimmed == ".vscode" && i+1 < len(parts):
			switch strings.TrimRight(parts[i+1], ". ") {
			case "tasks.json", "launch.json", "settings.json":
				return true
			}
		}
	}
	return false
}

// classifyConnectorTool covers tools that come from MCP connectors. The agent
// harness does not know what they do, so the name is the only evidence. A
// name that reads as a lookup is an observation; a name that reads as sending,
// deleting or paying is an external effect; everything else is an execution.
func classifyConnectorTool(tool string, input json.RawMessage) Action {
	name := strings.ToLower(tool)
	summary := "Connector tool: " + tool
	detail := ""
	if len(input) > 0 && len(input) < 2000 {
		detail = string(input)
	}

	for _, word := range externalWords {
		if strings.Contains(name, word) {
			return Action{Tool: tool, Tier: TierExternal, Summary: summary, Detail: detail}
		}
	}
	for _, prefix := range lookupPrefixes {
		if strings.HasPrefix(name, prefix) {
			return Action{Tool: tool, Tier: TierObserve, Summary: summary}
		}
	}
	return Action{Tool: tool, Tier: TierExecute, Summary: summary, Detail: detail, Pattern: "tool:" + tool}
}

var (
	externalWords  = []string{"send", "delete", "remove", "post", "publish", "pay", "purchase", "transfer", "forward", "reply", "upload", "share", "drop", "kill", "shutdown", "install", "uninstall"}
	lookupPrefixes = []string{"read", "list", "get", "search", "find", "query", "show", "view", "describe", "inspect", "fetch_status", "screenshot"}
)

// --- run_command -------------------------------------------------------------

// ClassifyCommand decides the tier of one shell command line.
func ClassifyCommand(command, cwd, workspace string) Action {
	command = strings.TrimSpace(command)
	act := Action{
		Tool:    "run_command",
		Tier:    TierExecute,
		Summary: "Run: " + clip(oneLine(command), 160),
		Detail:  command,
	}
	if command == "" {
		return act
	}

	// 2>&1 and >/dev/null are harmless, and splitting on the & inside 2>&1
	// would turn "1" into a command of its own.
	parts := splitCompound(stripSafeRedirects(command))
	compound := len(parts) > 1 || hasShellMeta(command)

	tier := TierObserve
	for _, part := range parts {
		if t := tierOfSimple(part, cwd, workspace); t > tier {
			tier = t
		}
	}
	// A command line is an observation only when every part is a known
	// read-only command (tierOfSimple returns Execute for anything it does not
	// recognise), so an unfamiliar program is never waved through.
	// Pipes into an interpreter run whatever came down the pipe.
	if pipesIntoInterpreter(command) && tier < TierReach {
		tier = TierReach
	}
	// Substitution, variable expansion and redirection are worked out by the
	// shell, after the words were read. `echo $(rm -rf x)` begins with a
	// lookup and runs a deletion; `cat a > b` begins with a lookup and writes a
	// file; `echo $API_KEY` prints a secret. The words of such a line are no
	// evidence of what it does, so it is never an observation.
	if hasUnsafeSyntax(command) && tier < TierExecute {
		tier = TierExecute
	}
	// A cwd that leaves the workspace is a reach, whatever the command is.
	if cwdEscapes(cwd, workspace) && tier < TierReach {
		tier = TierReach
	}

	act.Tier = tier
	if tier == TierExecute && !compound {
		act.Pattern = commandPattern(command)
	}
	return act
}

// tierOfSimple classifies one command with no operators in it.
func tierOfSimple(part, cwd, workspace string) Tier {
	fields := fieldsOf(part)
	if len(fields) == 0 {
		return TierObserve
	}
	lower := strings.ToLower(strings.Join(fields, " "))
	for _, re := range externalPatterns {
		if re.MatchString(lower) {
			return TierExternal
		}
	}
	for _, re := range reachPatterns {
		if re.MatchString(lower) {
			return TierReach
		}
	}
	if pathEscapes(fields[1:], workspace) {
		return TierReach
	}
	if isReadOnlyCommand(part) {
		return TierObserve
	}
	return TierExecute
}

var externalPatterns = mustCompile(
	`^git\s+(push|remote\s+(add|set-url))\b`,
	`^(npm|pnpm|yarn)\s+(publish|unpublish|deprecate|login|adduser|owner|access)\b`,
	`^(cargo)\s+(publish|yank|login|owner)\b`,
	`^(twine)\s+upload\b`,
	`^(docker|podman)\s+(push|login)\b`,
	`^(kubectl|helm)\s+(apply|delete|create|replace|patch|install|upgrade|uninstall|rollout|scale)\b`,
	`^terraform\s+(apply|destroy|import)\b`,
	`^(gh|glab)\s+\w+\s+(create|merge|delete|close|edit|comment|release|upload)\b`,
	`^(ssh|scp|sftp|rsync|ftp|telnet|nc|ncat)\b`,
	`^(aws|gcloud|az|doctl|heroku|vercel|netlify|fly|flyctl|wrangler)\b`,
	`^(curl|wget|http|https|xh)\b.*(\s-x\s*(post|put|patch|delete)|--request\s*(post|put|patch|delete)|\s-d\b|--data|--form|\s-f\b|--upload-file|\s-t\s)`,
	`^(invoke-webrequest|invoke-restmethod|iwr|irm)\b.*-method\s+(post|put|patch|delete)`,
	`^(send-mailmessage|mail|mailx|sendmail)\b`,
	`^(sc|net)\s+(start|stop|use)\b`,
)

var reachPatterns = mustCompile(
	`^sudo\b`,
	`^(su|runas|doas)\b`,
	`^rm\s+(-\w*r\w*|--recursive)\b`,
	`^rm\s+.*(-\w*r\w*f|-\w*f\w*r)`,
	`^(del|erase|rd|rmdir)\b.*(/s|/q)`,
	`^remove-item\b.*-recurse`,
	`^(format|diskpart|mkfs\S*|fdisk|parted|dd)\b`,
	`^(shutdown|reboot|halt|poweroff|restart-computer|stop-computer|logoff)\b`,
	`^(reg|regedit|setx|set-itemproperty|new-itemproperty|remove-itemproperty)\b`,
	`^(chmod|chown|icacls|takeown|attrib)\b.*(-r|/t|/s|-recurse)`,
	`^(taskkill|kill|killall|pkill|stop-process)\b`,
	`^(set-executionpolicy|netsh|bcdedit|schtasks|crontab|launchctl|systemctl)\b`,
	`^(powershell|pwsh|cmd|bash|sh|zsh)\b.*(-enc\b|-encodedcommand|-e\s+[a-z0-9+/=]{20,})`,
	`^(iex|invoke-expression)\b`,
	`^git\s+(reset\s+--hard|clean\s+-\w*f|checkout\s+--\s+\.|branch\s+-d|push\s+.*--force)`,
	`^(npm|pnpm|yarn)\s+(install|i|add)\s+(-g|--global)\b`,
	`^(pip|pip3)\s+install\b.*(--user|--break-system-packages)`,
	`^(winget|choco|scoop|apt|apt-get|brew|dnf|yum|pacman)\s+(install|uninstall|remove|upgrade)\b`,
)

func mustCompile(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

// readOnlyPrograms are commands that only look. They are matched on the whole
// line so `git status` is a lookup and `git commit` is not.
var readOnlyLines = mustCompile(
	`^(ls|dir|pwd|cd|echo|whoami|hostname|date|get-date|get-location|tree|wc|which|where|get-childitem|gci)(\s|$)`,
	`^(cat|type|head|tail|more|less|get-content|gc|findstr|grep|rg|select-string)\s`,
	`^git\s+(status|log|diff|show|rev-parse|remote\s+-v|describe|ls-files|blame|shortlog|tag\s+-l|config\s+--get)\b`,
	// `git branch name` creates a branch and `git branch -D` deletes one, so
	// only the listing forms count as a lookup.
	`^git\s+branch(\s+(-a|-r|-v|-vv|--all|--remotes|--list|--show-current))*\s*$`,
	`^(node|npm|pnpm|yarn|go|python|python3|pip|pip3|cargo|rustc|java|dotnet|git|docker|tsc)\s+(-v|--version|version)\s*$`,
	`^go\s+(list|env|vet)\b`,
	`^(npm|pnpm|yarn)\s+(ls|list|outdated|why)\b`,
)

// unsafeReadFlags turn an otherwise harmless lookup into something that runs a
// program or writes a file: a tool wrapper, a preprocessor, an output file, an
// external diff driver, a changed Go environment.
var unsafeReadFlags = regexp.MustCompile(
	`(^|\s)(-{1,2}toolexec|-{1,2}vettool|--pre|--pre-glob|--output|--ext-diff|--textconv|--exec|-exec|--open-files-in-pager)(=|\s|$)` +
		`|^go\s+env\b.*\s-[wu]\b|^tree\b.*\s-o\b`)

func isReadOnlyCommand(part string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(part))
	if unsafeReadFlags.MatchString(trimmed) {
		return false
	}
	for _, re := range readOnlyLines {
		if re.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// --- helpers ----------------------------------------------------------------

var compoundSplit = regexp.MustCompile(`&&|\|\||;|\||\n|&`)

func splitCompound(command string) []string {
	var out []string
	for _, p := range compoundSplit.Split(command, -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// hasShellMeta reports redirection, substitution or chaining. Such a line is
// never remembered as a pattern: what was approved would not be what runs next.
func hasShellMeta(command string) bool {
	return strings.ContainsAny(command, "&|;<>`\n") || strings.Contains(command, "$(") ||
		strings.Contains(command, "${") || shellExpansion.MatchString(command)
}

// safeRedirect is a redirection that only joins streams or discards output.
var safeRedirect = regexp.MustCompile(`(?i)\d*>&\d+|\d*>>?\s*(?:/dev/null|nul)\b`)

// shellExpansion is anything the shell replaces before running: $VAR, ${VAR},
// $(cmd), `cmd`, process substitution, PowerShell @(...) and %VAR%. A digit after
// the dollar sign ($1 in an awk program) is not an expansion worth asking about.
var shellExpansion = regexp.MustCompile("\\$[A-Za-z_{(]|`|<\\(|>\\(|@\\(|%[A-Za-z_][A-Za-z0-9_]*%")

func stripSafeRedirects(command string) string {
	return safeRedirect.ReplaceAllString(command, " ")
}

// hasUnsafeSyntax reports expansion, substitution or a redirection that writes
// to a file or reads a here-document, once the harmless redirections are set aside.
func hasUnsafeSyntax(command string) bool {
	c := stripSafeRedirects(command)
	return shellExpansion.MatchString(c) || strings.Contains(c, ">") || strings.Contains(c, "<<")
}

var interpreterPipe = regexp.MustCompile(`(?i)\|\s*(sudo\s+)?(sh|bash|zsh|fish|dash|python3?|node|perl|ruby|iex|invoke-expression|powershell|pwsh|cmd)\b`)

func pipesIntoInterpreter(command string) bool {
	return interpreterPipe.MatchString(command)
}

func fieldsOf(part string) []string {
	return strings.Fields(part)
}

// pathEscapes reports whether any argument names a place outside the
// workspace: a parent-directory climb, a home-directory shorthand, or an
// absolute path that is not under the workspace root.
func pathEscapes(args []string, workspace string) bool {
	for _, raw := range args {
		arg := strings.Trim(raw, `"'`)
		if arg == "" || strings.HasPrefix(arg, "-") && !strings.ContainsAny(arg, `/\`) {
			continue
		}
		// --flag=/some/path
		if i := strings.Index(arg, "="); i > 0 && strings.HasPrefix(arg, "-") {
			arg = arg[i+1:]
		}
		lower := strings.ToLower(arg)
		switch {
		case arg == ".." || strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, `..\`) ||
			strings.Contains(arg, "/../") || strings.Contains(arg, `\..\`) ||
			strings.HasSuffix(arg, "/..") || strings.HasSuffix(arg, `\..`):
			return true
		case strings.HasPrefix(arg, "~") || strings.HasPrefix(lower, "$home") ||
			strings.HasPrefix(lower, "%userprofile%") || strings.HasPrefix(lower, "$env:") ||
			strings.HasPrefix(lower, "%appdata%") || strings.HasPrefix(lower, "%localappdata%") ||
			strings.HasPrefix(lower, "%systemroot%") || strings.HasPrefix(lower, "%windir%"):
			return true
		case isAbsolute(arg):
			if !within(workspace, arg) {
				return true
			}
		}
	}
	return false
}

func isAbsolute(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

// within is a lexical containment check. The caller treats "cannot tell" as
// outside, which is the cautious answer.
func within(root, p string) bool {
	if root == "" {
		return false
	}
	r := normalize(root)
	c := normalize(p)
	return c == r || strings.HasPrefix(c, r+"/")
}

func normalize(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	p = filepath.ToSlash(filepath.Clean(p))
	p = strings.TrimRight(p, "/")
	if runtime.GOOS == "windows" || (len(p) > 1 && p[1] == ':') {
		p = strings.ToLower(p)
	}
	return p
}

func cwdEscapes(cwd, workspace string) bool {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || cwd == "." {
		return false
	}
	return pathEscapes([]string{cwd}, workspace)
}

// commandPattern is what "allow this command in this workspace" remembers: the
// program and its first sub-command, so approving `npm install` covers
// `npm install left-pad` but not `npm publish`.
func commandPattern(command string) string {
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 {
		return ""
	}
	prog := filepath.Base(strings.Trim(fields[0], `"'`))
	prog = strings.TrimSuffix(prog, ".exe")
	n := 1
	if len(fields) > 1 && !strings.HasPrefix(fields[1], "-") && !strings.ContainsAny(fields[1], `/\.`) {
		n = 2
	}
	parts := append([]string{prog}, fields[1:n]...)
	return strings.Join(parts, " ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
