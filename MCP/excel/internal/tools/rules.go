package tools

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// This file turns the words a model used for a rule — "greater_than", "list",
// "colour scale" — into the option records the spreadsheet library wants, and
// refuses the combinations that would produce a rule Excel cannot read.

// conditionalRule builds a conditional formatting rule. The library's Type and
// Criteria are its own vocabulary rather than Excel's, so the mapping lives
// here, in one place, with the friendly names on the outside.
func conditionalRule(file *excelize.File, rule, value, value2, fill, font string, bold bool, midColor, barColor string, stopIfTrue bool) (excelize.ConditionalFormatOptions, error) {
	options := excelize.ConditionalFormatOptions{StopIfTrue: stopIfTrue}

	needsValue := true
	switch rule {
	case "greater_than":
		options.Type, options.Criteria = "cell", ">"
	case "greater_than_or_equal":
		options.Type, options.Criteria = "cell", ">="
	case "less_than":
		options.Type, options.Criteria = "cell", "<"
	case "less_than_or_equal":
		options.Type, options.Criteria = "cell", "<="
	case "equal":
		options.Type, options.Criteria = "cell", "="
	case "not_equal":
		options.Type, options.Criteria = "cell", "!="
	case "between", "not_between":
		options.Type = "cell"
		options.Criteria = "between"
		if rule == "not_between" {
			options.Criteria = "not between"
		}
		if value == "" || value2 == "" {
			return options, fmt.Errorf("%s needs both value and value2", rule)
		}
		options.MinValue, options.MaxValue = value, value2
		needsValue = false
	case "contains_text":
		options.Type, options.Criteria = "text", "containing"
	case "not_contains_text":
		options.Type, options.Criteria = "text", "not containing"
	case "begins_with":
		options.Type, options.Criteria = "text", "begins with"
	case "ends_with":
		options.Type, options.Criteria = "text", "ends with"
	case "color_scale":
		options.Type = "2_color_scale"
		// A scale carries its own colours, but SetConditionalFormat still
		// validates the criteria, so it has to be given one it knows.
		options.Criteria = "="
		options.MinType, options.MaxType = "min", "max"
		options.MinColor, options.MaxColor = "F8696B", "63BE7B"
		options.MinValue, options.MaxValue = "0", "0"
		if midColor != "" {
			options.Type = "3_color_scale"
			options.MidType, options.MidValue = "percentile", "50"
			options.MidColor, _ = workbook.HexColor(midColor, "mid_color")
		}
		if fill != "" {
			options.MinColor, _ = workbook.HexColor(fill, "fill_color")
		}
		if barColor != "" {
			options.MaxColor, _ = workbook.HexColor(barColor, "bar_color")
		}
		needsValue = false
	case "data_bar":
		options.Type = "data_bar"
		// A bar carries its own colour too, but the same validation applies.
		options.Criteria = "="
		options.MinType, options.MaxType = "min", "max"
		options.MinValue, options.MaxValue = "0", "0"
		options.BarColor = "638EC6"
		options.BarSolid = true
		if barColor != "" {
			color, err := workbook.HexColor(barColor, "bar_color")
			if err != nil {
				return options, err
			}
			options.BarColor = color
		}
		if fill != "" {
			color, err := workbook.HexColor(fill, "fill_color")
			if err != nil {
				return options, err
			}
			options.BarColor = color
		}
		needsValue = false
	default:
		return options, fmt.Errorf("no rule called %q", rule)
	}

	if needsValue && value == "" {
		return options, fmt.Errorf("%s needs a value to compare against", rule)
	}
	if needsValue {
		options.Value = value
	}

	// A rule that colours a cell needs a style to colour it with. A scale and a
	// bar carry their own colours, so they are left alone.
	if rule != "color_scale" && rule != "data_bar" {
		if !bold && fill == "" && font == "" {
			return options, fmt.Errorf("%s needs something to show a match with: at least one of fill_color, font_color or bold", rule)
		}
		id, err := matchStyle(file, fill, font, bold)
		if err != nil {
			return options, err
		}
		options.Format = &id
	}

	return options, nil
}

// matchStyle builds the look a matching cell takes.
func matchStyle(file *excelize.File, fill, font string, bold bool) (int, error) {
	spec := &excelize.Style{}
	if bold || font != "" {
		fontSpec := &excelize.Font{Bold: bold}
		if font != "" {
			color, err := workbook.HexColor(font, "font_color")
			if err != nil {
				return 0, err
			}
			fontSpec.Color = color
		}
		spec.Font = fontSpec
	}
	if fill != "" {
		color, err := workbook.HexColor(fill, "fill_color")
		if err != nil {
			return 0, err
		}
		spec.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{color}}
	}
	id, err := file.NewStyle(spec)
	if err != nil {
		return 0, fmt.Errorf("build the rule's style: %w", err)
	}
	return id, nil
}

// validationOperators maps the words the tool accepts to the words Excel stores.
var validationOperators = map[string]string{
	"between":               "between",
	"not_between":           "notBetween",
	"equal":                 "equal",
	"not_equal":             "notEqual",
	"greater_than":          "greaterThan",
	"less_than":             "lessThan",
	"greater_than_or_equal": "greaterThanOrEqual",
	"less_than_or_equal":    "lessThanOrEqual",
}

// validationKinds maps the tool's words to the types Excel stores.
var validationKinds = map[string]string{
	"list":        "list",
	"whole":       "whole",
	"decimal":     "decimal",
	"date":        "date",
	"time":        "time",
	"text_length": "textLength",
	"custom":      "custom",
}

func validationRule(kind string, values []string, formula1, formula2, operator string, allowBlank bool, title, message, inputTitle, inputMessage string) (*excelize.DataValidation, error) {
	stored, known := validationKinds[kind]
	if !known {
		return nil, fmt.Errorf("no rule kind called %q", kind)
	}

	rule := &excelize.DataValidation{
		Type:       stored,
		AllowBlank: allowBlank,
	}

	switch kind {
	case "list":
		switch {
		case len(values) > 0:
			// Excel wants the choices as one quoted, comma-separated string, and
			// a choice that itself holds a comma would break the list, so it is
			// refused with the fix rather than written unreadable.
			for _, choice := range values {
				if strings.ContainsAny(choice, `,"`) {
					return nil, fmt.Errorf("the list choice %q holds a comma or a quote: put the choices in a range on the sheet and pass that range as formula1 instead", choice)
				}
			}
			rule.Formula1 = `"` + strings.Join(values, ",") + `"`
		case formula1 != "":
			rule.Formula1 = formula1
		default:
			return nil, fmt.Errorf("a list needs the choices: pass values, or formula1 holding a range")
		}
	default:
		if formula1 == "" {
			return nil, fmt.Errorf("%s needs formula1: the limit, a number or a cell such as $B$1", kind)
		}
		rule.Formula1 = formula1
		if operator != "" && kind != "custom" {
			rule.Operator = validationOperators[operator]
		}
		if (operator == "between" || operator == "not_between") && formula2 == "" {
			return nil, fmt.Errorf("%s needs formula2 as well: the upper limit", operator)
		}
		if formula2 != "" {
			rule.Formula2 = formula2
		}
	}

	if title != "" || message != "" {
		rule.ShowErrorMessage = true
		style := "stop"
		rule.ErrorStyle = &style
		if title != "" {
			rule.ErrorTitle = &title
		}
		if message != "" {
			rule.Error = &message
		}
	}
	if inputTitle != "" || inputMessage != "" {
		rule.ShowInputMessage = true
		if inputTitle != "" {
			rule.PromptTitle = &inputTitle
		}
		if inputMessage != "" {
			rule.Prompt = &inputMessage
		}
	}

	return rule, nil
}

// describeRule is what the model is told afterwards, in its own words.
func describeRule(kind string, values []string) string {
	if kind == "list" && len(values) > 0 {
		return fmt.Sprintf("one of %s", strings.Join(values, ", "))
	}
	return kind
}
