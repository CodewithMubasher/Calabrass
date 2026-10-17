package agent

import (
	"strings"
	"testing"
)

// TestThePromptNamesTheConnectorsAndWhatTheyReach is the fix for a model that was
// handed the tools that reach the user's desktop and answered by reading the
// project: the prompt has to say that the machine is there, and which tools
// touch it.
func TestThePromptNamesTheConnectorsAndWhatTheyReach(t *testing.T) {
	prompt := SystemPrompt(nil, "You are a test.", []Connector{{
		Name:         "win-control",
		Instructions: "These tools act on the computer you are running on.",
		Tools:        []string{"open_app", "play_music"},
	}})

	for _, want := range []string{
		"win-control",
		"These tools act on the computer you are running on.",
		"open_app, play_music",
		"use them instead of looking through the project",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not mention %q:\n%s", want, prompt)
		}
	}
}

// TestThePromptSaysNothingAboutAMachineWithoutConnectors: with no connectors the
// tools really are all about the project, and describing a desktop would be
// describing tools the model does not have.
func TestThePromptSaysNothingAboutAMachineWithoutConnectors(t *testing.T) {
	prompt := SystemPrompt(nil, "You are a test.", nil)
	if strings.Contains(prompt, "the connectors below") {
		t.Errorf("the prompt describes connectors that are not there:\n%s", prompt)
	}
}

// TestTheProjectParagraphNeedsAProject: the machine section stands on its own,
// so nothing claims a project root that does not exist.
func TestTheProjectParagraphNeedsAProject(t *testing.T) {
	prompt := SystemPrompt(nil, "You are a test.", []Connector{{Name: "win-control"}})
	if strings.Contains(prompt, "Project root:") {
		t.Errorf("the prompt names a project root when there is no project:\n%s", prompt)
	}
}

func TestThePromptStillDescribesTheProject(t *testing.T) {
	ws, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	prompt := SystemPrompt(ws, "You are a test.", []Connector{{Name: "win-control", Tools: []string{"open_app"}}})

	for _, want := range []string{"Project root: " + ws.Root(), "win-control", "open_app", "Tool use:"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not mention %q:\n%s", want, prompt)
		}
	}
}
