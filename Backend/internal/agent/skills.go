package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zendev-sh/goai"
)

// A skill is a folder under ~/.calabrass/skills holding a SKILL.md: a set of
// instructions one worker reads before it starts. Skills are global to the
// machine rather than per workspace, because the same way of building a
// frontend should follow the person from project to project.
const (
	// CalabrassDirName is the folder in the user's home that holds local data.
	CalabrassDirName = ".calabrass"
	// SkillsDirName is the folder inside it holding one folder per skill.
	SkillsDirName = "skills"
	// SkillFileName is the document inside a skill folder.
	SkillFileName = "SKILL.md"
)

// Skill is one installable set of instructions, as the picker lists it.
type Skill struct {
	// Name is the folder name, which is also what a worker is told to read.
	Name string `json:"name"`
	// Description is the first heading or line of its SKILL.md.
	Description string `json:"description"`
	// Path is the absolute path to the SKILL.md, for display.
	Path string `json:"path"`
}

// CalabrassDir is ~/.calabrass, created on first use.
func CalabrassDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, CalabrassDirName), nil
}

// SkillsRoot is ~/.calabrass/skills. It is created when it is missing so the
// picker has somewhere to read and a person has somewhere to drop a skill.
func SkillsRoot() (string, error) {
	dir, err := CalabrassDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(dir, SkillsDirName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create skills directory: %w", err)
	}
	return root, nil
}

// ListSkills returns every folder under the skills root that holds a SKILL.md,
// alphabetically. A folder without that document is not a skill and is skipped.
func ListSkills() ([]Skill, error) {
	root, err := SkillsRoot()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Skill{}, nil
		}
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(root, name, SkillFileName)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		skills = append(skills, Skill{
			Name:        name,
			Description: skillDescription(path),
			Path:        path,
		})
	}

	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills, nil
}

// skillDescription reads the opening of a SKILL.md: its first heading when it
// has one, otherwise its first non-empty line. Reading the whole document for a
// one-line label would be wasteful, so only the head is read.
func skillDescription(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, 4096)
	n, _ := file.Read(buf)
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line != "" {
			return truncateWords(line, 120)
		}
	}
	return ""
}

// ReadSkill returns the text of one skill's SKILL.md. The name is a single
// folder name: anything with a separator or a parent hop is refused, so the
// tool cannot reach outside the skills root.
func ReadSkill(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("a skill name is required")
	}
	if name != filepath.Base(name) || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("%q is not a skill name", name)
	}

	root, err := SkillsRoot()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, name, SkillFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("skill %q has no %s", name, SkillFileName)
		}
		return "", err
	}
	if looksBinary(data) {
		return "", fmt.Errorf("skill %q is not a text document", name)
	}
	return string(data), nil
}

// readSkillArgs is the wire shape of read_skill.
type readSkillArgs struct {
	Skill string `json:"skill" jsonschema:"description=The skill folder name to read, exactly as listed in the task"`
}

// ReadSkillTool is the one tool a worker uses to load its instructions. Skills
// deliberately do not ride in the worker's system prompt: the worker is told
// which skills it owns and must read them, so what it follows is the file on
// disk rather than a copy that can drift.
func ReadSkillTool() goai.Tool {
	return goai.NewTool("read_skill",
		"Read one of your skill files before starting work. Call it once for every skill listed in your task. A skill is a folder under ~/.calabrass/skills with a SKILL.md.",
		func(ctx context.Context, args readSkillArgs) (string, error) {
			text, err := ReadSkill(args.Skill)
			if err != nil {
				return "", err
			}
			return "# " + args.Skill + "\n\n" + text, nil
		})
}

// SkillInstruction is the block a worker's prompt carries: which skills it owns
// and the rule that none of them is optional.
func SkillInstruction(skills []string) string {
	var b strings.Builder
	if len(skills) == 0 {
		b.WriteString("You have no skills assigned. Do the task with the tools you have.")
		return b.String()
	}

	b.WriteString("Before you do anything else, you MUST read every skill file assigned to you with the read_skill tool:\n")
	for _, skill := range skills {
		name := strings.TrimSpace(skill)
		if name == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", name)
	}
	b.WriteString("Read all of them first, in one step, then follow them exactly while doing the task. ")
	b.WriteString("A skill that is listed but cannot be read is a blocker: report it instead of guessing what it says.")
	return b.String()
}
