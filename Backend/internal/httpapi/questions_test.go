package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// The point of the tool: a model that needs something only the user knows has
// to be able to ask, in either mode, and the turn's tool list is where that
// either happens or does not.
func TestAChatTurnIsOfferedTheAskQuestionsTool(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st, log: slog.Default()}
	setup := s.setupTurn(context.Background(), nil, "chat-1")

	for _, tool := range setup.tools {
		if tool.Name == "ask_questions" {
			return
		}
	}
	t.Fatal("a chat turn was not offered the ask_questions tool")
}

// What the model is told after it has asked: that the answers are on their way,
// and what was asked.
func TestAskQuestionsResult(t *testing.T) {
	out, err := askQuestionsResult(askQuestionsArgs{
		Questions: []askQuestion{
			{Question: "Which framework?"},
			{Question: "Which database?", Options: []string{"Postgres", "Postgres", "  "}},
		},
	})
	if err != nil {
		t.Fatalf("ask questions: %v", err)
	}

	for _, want := range []string{
		"Asked the user 2 questions.",
		"1. Which framework?",
		"2. Which database? (Postgres)",
		"wait for them",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the result does not say %q:\n%s", want, out)
		}
	}
}

// A call with nothing in it is a mistake, not a question.
func TestAskQuestionsResultNeedsAQuestion(t *testing.T) {
	if _, err := askQuestionsResult(askQuestionsArgs{}); err == nil {
		t.Fatal("an empty set of questions was accepted")
	}
	if _, err := askQuestionsResult(askQuestionsArgs{
		Questions: []askQuestion{{Question: "   "}},
	}); err == nil {
		t.Fatal("a blank question was accepted")
	}
}

// Models do not always use the names the tool describes. One that writes a
// questionnaire from memory sends "description" and "values", and its call has
// to be read as the questions it plainly is.
func TestAskQuestionsReadsTheNamesModelsWrite(t *testing.T) {
	var args askQuestionsArgs
	err := json.Unmarshal([]byte(`{
		"questions": [
			{"description": "What are your main goals?", "type": "text"},
			{"description": "How do you prefer to work?", "type": "select",
			 "values": ["Take the lead", "Suggest and review"]}
		]
	}`), &args)
	if err != nil {
		t.Fatalf("read the call: %v", err)
	}

	if len(args.Questions) != 2 {
		t.Fatalf("read %d questions, want 2", len(args.Questions))
	}
	if args.Questions[0].Question != "What are your main goals?" {
		t.Errorf("the first question reads %q", args.Questions[0].Question)
	}
	if len(args.Questions[0].Options) != 0 {
		t.Errorf("a free-text question came with choices: %v", args.Questions[0].Options)
	}
	if args.Questions[1].Question != "How do you prefer to work?" {
		t.Errorf("the second question reads %q", args.Questions[1].Question)
	}
	if len(args.Questions[1].Options) != 2 {
		t.Errorf("the second question has %d choices, want 2", len(args.Questions[1].Options))
	}

	out, err := askQuestionsResult(args)
	if err != nil {
		t.Fatalf("ask questions: %v", err)
	}
	for _, want := range []string{
		"Asked the user 2 questions.",
		"1. What are your main goals?",
		"2. How do you prefer to work? (Take the lead / Suggest and review)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the result does not say %q:\n%s", want, out)
		}
	}
}

// The rule has to be in the prompt the model is given, in every kind of turn.
// Without it the model answers in plain text, and the user never gets a form to
// fill in.
func TestTheTurnPromptSaysHowToAskTheUser(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ws, err := agent.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	s := &Server{store: st, log: slog.Default()}
	turns := map[string]turnSetup{
		"with no project": s.setupTurn(context.Background(), nil, "chat-1"),
		"with a project":  s.setupTurn(context.Background(), ws, "chat-1"),
	}

	for name, setup := range turns {
		for _, want := range []string{
			"ask_questions",
			"MUST call the ask_questions tool",
			"never ask a question in your own text",
			`"question"`,
			`"options"`,
		} {
			if !strings.Contains(setup.system, want) {
				t.Errorf("the prompt of a turn %s does not say %q:\n%s", name, want, setup.system)
			}
		}
	}
}

// A list of plain strings is a list of questions, and a bare list is a list of
// questions too.
func TestAskQuestionsReadsAListThatIsNotUnderAKey(t *testing.T) {
	var plain askQuestionsArgs
	if err := json.Unmarshal([]byte(`{"questions": ["What should I build?"]}`), &plain); err != nil {
		t.Fatalf("read the call: %v", err)
	}
	if len(plain.Questions) != 1 || plain.Questions[0].Question != "What should I build?" {
		t.Fatalf("read %+v, want one question", plain.Questions)
	}

	var bare askQuestionsArgs
	if err := json.Unmarshal([]byte(`[{"question": "What should I build?"}]`), &bare); err != nil {
		t.Fatalf("read the call: %v", err)
	}
	if len(bare.Questions) != 1 || bare.Questions[0].Question != "What should I build?" {
		t.Fatalf("read %+v, want one question", bare.Questions)
	}
}
