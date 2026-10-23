package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultSkills are the skills the app installs on first run, so a fresh
// account already has a useful frontend standard to hand a worker. They are
// ordinary files in ~/.calabrass/skills afterwards: editing or deleting one is
// the person's choice, and nothing here puts it back.
var defaultSkills = map[string]string{
	"frontend-design": `# Frontend design

Use this skill whenever you build or change a user interface. It is the
standard the result is judged against.

## Start from the job

- Write down what the screen is for and who uses it. One sentence each.
- List the three actions that matter most. They get the space, contrast, and
  position. Everything else is secondary and should look secondary.
- Work mobile first: one column, one action per row, then add width only where
  it makes the job easier.

## Layout

- Use a spacing scale and keep to it: 4, 8, 12, 16, 24, 32, 48, 64px.
- Align to a grid. If two things should line up, they line up exactly, not
  nearly.
- Group related controls in one surface. Every extra box, border, and line
  costs attention, so remove one before adding one.
- Keep a single visual hierarchy: page title, section title, body, metadata.
  Each step down is clearly smaller or dimmer than the one above it.

## Typography

- Body text 15-16px with 1.5-1.65 line height; never below 13px for anything a
  person must read.
- Cap the measure at about 65-75 characters. Full-width paragraphs are unreadable.
- Use no more than two weights and two sizes per component. Use weight, not
  colour, for emphasis first.
- Never use decorative fonts for interface text.

## Colour

- Build from tokens, not one-off hex values. Define background, surface, border,
  text, muted text, and one accent.
- Meet WCAG AA contrast: 4.5:1 for body text, 3:1 for large text and controls.
- The accent marks action. If everything is the accent, nothing is.
- Test the dark and light surfaces you actually ship, not the ones you meant to.

## States

Every control needs all of them: default, hover, focus-visible, active,
disabled, loading, empty, and error.

- Focus-visible must be obvious without a mouse: a ring or an outline, never
  nothing.
- Disabled is visibly disabled and still readable.
- Empty states say what would be here and how to put it there.
- Errors say what happened, where, and what to do next. Never a bare "Error".
- Loading uses a skeleton where the shape is known, a spinner where it is not.

## Motion

- Animate only to explain a change: where something came from, where it went,
  that it is working. 120-240ms is enough for most interface motion.
- Respect prefers-reduced-motion.
- Never animate layout that the person is trying to read.

## Accessibility

- Every input has a real label. Placeholders are hints, not labels.
- Every icon-only button has an aria-label.
- Everything works with the keyboard, in a sensible tab order.
- Images have alt text; decorative images have empty alt.
- Announce async results and errors with the right live region.

## Implementation

- Match the project's existing stack, component library, tokens, and file
  layout. Do not introduce a second design system.
- Reuse existing components before writing new ones. Extend them with props
  rather than copying them.
- Keep components small and named for what they show. Move state up only when
  two siblings need it.
- No inline styles for anything that belongs in the scale; no magic numbers.
- Check the result in the narrowest and widest layout you support before
  calling it done.

## Finish checklist

- [ ] The primary action is obvious within one second.
- [ ] Spacing uses the scale; nothing is off by a pixel.
- [ ] Both themes, if the app has them, read correctly.
- [ ] Hover, focus, disabled, loading, empty, and error states exist.
- [ ] Keyboard-only use works.
- [ ] No text below 13px, no contrast failure, no horizontal scroll on mobile.
- [ ] The project's lint, typecheck, and build pass.
`,
	"project-analysis": `# Project analysis

Use this skill whenever you are asked to understand, map, or explain a codebase.
You do not write code — you read, trace, and report.

## How to explore

- **Start from the entry points**: main, routes, handlers, public APIs. Follow
  imports and references from there.
- **Search before you walk**: use grep to find a definition or a call site, and
  glob to find files by name. Prefer a narrow pattern over a repository-wide
  scan.
- **Read only the important files**. A large project has thousands of files;
  you cannot read them all. Focus on configuration, module boundaries, and the
  call chains relevant to the question.
- **Call tools in parallel** when you need to read multiple files, list multiple
  directories, or search for multiple patterns. Do it in one turn — the tools
  support concurrent calls.

## What to look for

- **Architecture**: layers, modules, boundaries, dependency direction.
- **Conventions**: naming, file layout, error handling, logging, testing patterns.
- **Data flow**: how a request becomes a response; where state lives; how it
  moves between components.
- **Extension points**: interfaces, plugins, hooks, middleware — where the
  project expects you to add behaviour.
- **Gotchas**: generated code, vendored deps, build-time codegen, platform-
  specific files, runtime-only behaviour.

## Reporting

- Lead with the answer: what you found, where, and what it means.
- Name the key files and symbols. Paths relative to the workspace root.
- If something could not be found, say exactly what you looked for and why it
  might not be there.
- Do not dump raw file contents unless asked. Summarise.

## Finish checklist

- [ ] The question is answered directly.
- [ ] Key files and symbols are named.
- [ ] Conventions and patterns are described.
- [ ] Gaps and uncertainties are acknowledged.
`,
}

// EnsureDefaultSkills installs the shipped skills that are not already there.
// It never overwrites one: a person who edited or removed a skill keeps what
// they chose.
func EnsureDefaultSkills() error {
	root, err := SkillsRoot()
	if err != nil {
		return err
	}

	for name, body := range defaultSkills {
		dir := filepath.Join(root, name)
		path := filepath.Join(dir, SkillFileName)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create skill %q: %w", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write skill %q: %w", name, err)
		}
	}
	return nil
}
