package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	goai "github.com/zendev-sh/goai"
)

// --- ask_questions ------------------------------------------------------------

// askQuestion is one question the model wants answered.
//
// The names in the JSON tags are the ones the model is told to use. The reader
// below also accepts the other names models reach for on their own, because a
// question that arrived under the wrong name is still a question.
type askQuestion struct {
	Question string   `json:"question,omitempty" jsonschema:"description=The question to put to the user, in your own words"`
	Options  []string `json:"options,omitempty" jsonschema:"description=The choices to offer when the answer is one of a fixed set; leave it out for a free-text answer"`
	Multiple bool     `json:"multiple,omitempty" jsonschema:"description=Whether more than one of the choices may be picked"`
}

// askQuestionsArgs is the wire shape of ask_questions: the questions, in the
// order they should be read.
type askQuestionsArgs struct {
	Questions []askQuestion `json:"questions" jsonschema:"description=The questions to ask, in the order they should be read. Each one is an object with a question and, when the answer is one of a fixed set, an options list"`
}

const (
	// askQuestionsMax is how many questions one call may carry. A form with
	// more than this in it stops being a form and starts being a wall.
	askQuestionsMax = 8
	// askQuestionMaxChars caps one question, so a model that writes a paragraph
	// instead of a question still produces something that fits on the screen.
	askQuestionMaxChars = 300
	// askOptionMaxChars caps one offered choice.
	askOptionMaxChars = 120
)

// askQuestionsTool lets the model ask the user for what it cannot work out for
// itself. It is the server's own tool rather than the workspace's: the
// questions are shown in a form above the composer, and the answers come back
// in the user's next message. Nothing is read or written on the way, so it is
// offered in every mode.
func (s *Server) askQuestionsTool() goai.Tool {
	return goai.NewTool("ask_questions",
		"Ask the user for what only they can decide or know: a requirement you cannot read from the project, a choice between approaches, a path or credential that is not there to find. "+
			"Call it with a list of questions, each one an object with a question and, when the answer is one of a fixed set, an options list: {\"questions\":[{\"question\":\"Which database should I use?\",\"options\":[\"Postgres\",\"SQLite\"]},{\"question\":\"What should the app be called?\"}]}. "+
			"Leave options out for a free-text answer, and set multiple to true when more than one option may be picked. "+
			"The questions are shown as a form above the composer and the answers arrive in the user's next message, so ask everything you need in one call and then stop: do not carry on with the task until the answers have come back. "+
			"Do not use it for anything you can find out yourself by reading the project or running a read-only command.",
		func(ctx context.Context, args askQuestionsArgs) (string, error) {
			return askQuestionsResult(args)
		})
}

// UnmarshalJSON reads one question from the names models write for it. The
// names the tool describes are "question" and "options"; "description" and
// "values" are what a model falls back on when it is writing a questionnaire
// from memory, and a question that arrived under another name is still one.
func (q *askQuestion) UnmarshalJSON(data []byte) error {
	var raw struct {
		Question    string   `json:"question"`
		Description string   `json:"description"`
		Text        string   `json:"text"`
		Prompt      string   `json:"prompt"`
		Options     []string `json:"options"`
		Values      []string `json:"values"`
		Choices     []string `json:"choices"`
		Multiple    bool     `json:"multiple"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	q.Question = firstQuestionText(raw.Question, raw.Description, raw.Text, raw.Prompt)
	q.Options = firstQuestionList(raw.Options, raw.Values, raw.Choices)
	q.Multiple = raw.Multiple
	return nil
}

// UnmarshalJSON reads the questions from the shapes a call arrives in: a list
// of objects under "questions", a bare list of objects, a list of plain
// strings, or a single question.
func (a *askQuestionsArgs) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}

	// The whole call is the list.
	if trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &a.Questions)
	}

	var raw struct {
		Questions json.RawMessage `json:"questions"`
		Question  json.RawMessage `json:"question"`
	}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}

	questions := bytes.TrimSpace(raw.Questions)
	if len(questions) == 0 || string(questions) == "null" {
		questions = bytes.TrimSpace(raw.Question)
	}
	if len(questions) == 0 || string(questions) == "null" {
		return nil
	}

	// A list of plain strings is a list of questions without any choices.
	if questions[0] == '[' {
		var plain []string
		if err := json.Unmarshal(questions, &plain); err == nil {
			for _, question := range plain {
				a.Questions = append(a.Questions, askQuestion{Question: question})
			}
			return nil
		}
		return json.Unmarshal(questions, &a.Questions)
	}

	// A single question.
	var one askQuestion
	if err := json.Unmarshal(questions, &one); err != nil {
		return err
	}
	a.Questions = append(a.Questions, one)
	return nil
}

// firstQuestionText is the first of the values that carries something.
func firstQuestionText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// firstQuestionList is the first of the lists that carries something.
func firstQuestionList(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

// askQuestionsResult is what the model is told once its questions are on the
// screen. It says to stop, because the answers arrive in the next message and
// anything the model does before then is work done on a guess.
func askQuestionsResult(args askQuestionsArgs) (string, error) {
	questions := make([]string, 0, len(args.Questions))
	for _, asked := range args.Questions {
		question := strings.TrimSpace(asked.Question)
		if question == "" {
			continue
		}
		if len(question) > askQuestionMaxChars {
			question = question[:askQuestionMaxChars] + "…"
		}
		if options := askOptions(asked.Options); len(options) > 0 {
			question = fmt.Sprintf("%s (%s)", question, strings.Join(options, " / "))
		}
		questions = append(questions, question)
	}

	if len(questions) == 0 {
		return "", errors.New("no question was given to ask: each question needs a question, and an options list when the answer is one of a fixed set")
	}
	if len(questions) > askQuestionsMax {
		questions = questions[:askQuestionsMax]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Asked the user %d question", len(questions))
	if len(questions) != 1 {
		b.WriteString("s")
	}
	b.WriteString(". The answers arrive in the user's next message, so stop this turn here and wait for them before you go on.\n")
	b.WriteString("The questions were:\n")
	for i, question := range questions {
		fmt.Fprintf(&b, "%d. %s\n", i+1, question)
	}
	return b.String(), nil
}

// askOptions keeps the choices a model offered, without the repeats and the
// ones too long to sit on a button.
func askOptions(options []string) []string {
	seen := make(map[string]bool, len(options))
	kept := make([]string, 0, len(options))
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" || seen[option] {
			continue
		}
		if len(option) > askOptionMaxChars {
			option = option[:askOptionMaxChars] + "…"
		}
		seen[option] = true
		kept = append(kept, option)
	}
	return kept
}
