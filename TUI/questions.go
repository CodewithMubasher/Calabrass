package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// askQuestion is one question the agent put to the user through ask_questions.
type askQuestion struct {
	Question string
	Options  []string
	Multiple bool
}

// answersMarker opens the message that carries the answers back. The web app
// uses the same line to tell an answered form from one still waiting.
const answersMarker = "Answers to your questions:"

// parseQuestions reads the arguments of an ask_questions call. Models do not
// always use the field names the tool describes, so the same alternatives the
// web app accepts are accepted here.
func parseQuestions(input string) []askQuestion {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return nil
	}
	if s, ok := parsed.(string); ok { // some providers send JSON inside a string
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			return nil
		}
	}

	var list []any
	switch v := parsed.(type) {
	case []any:
		list = v
	case map[string]any:
		raw := v["questions"]
		if raw == nil {
			raw = v["question"]
		}
		switch r := raw.(type) {
		case []any:
			list = r
		case map[string]any:
			list = []any{r}
		}
	}

	var out []askQuestion
	for _, entry := range list {
		switch e := entry.(type) {
		case string:
			if q := strings.TrimSpace(e); q != "" {
				out = append(out, askQuestion{Question: q})
			}
		case map[string]any:
			q := firstString(e["question"], e["description"], e["text"])
			if q == "" {
				continue
			}
			multi, _ := e["multiple"].(bool)
			out = append(out, askQuestion{
				Question: q,
				Options:  firstStringList(e["options"], e["values"], e["choices"]),
				Multiple: multi,
			})
		}
	}
	return out
}

func firstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstStringList(vals ...any) []string {
	for _, v := range vals {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		var kept []string
		for _, item := range arr {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				kept = append(kept, strings.TrimSpace(s))
			}
		}
		if len(kept) > 0 {
			return kept
		}
	}
	return nil
}

// formatAnswers is the message that returns the user's answers to the agent.
// The shape matches formatAnswers in Frontend/src/lib/questionnaire.ts.
func formatAnswers(qs []askQuestion, answers []string) string {
	var b strings.Builder
	b.WriteString(answersMarker)
	for i, q := range qs {
		answer := ""
		if i < len(answers) {
			answer = strings.TrimSpace(answers[i])
		}
		b.WriteString("\n")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(q.Question)
		if answer != "" {
			b.WriteString(" → " + answer)
		} else {
			b.WriteString(" → (no answer)")
		}
	}
	return b.String()
}

// pendingQuestions finds the questions of the last assistant message that
// asked some and has not been answered yet.
func pendingQuestions(msgs []Message) []askQuestion {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		switch {
		case m.Role == RoleNotice:
			continue
		case m.Role == RoleUser:
			return nil // the last real message is the user's: nothing is waiting
		}
		return m.Questions
	}
	return nil
}
