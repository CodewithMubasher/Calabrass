package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// chartTypes are the charts a model may ask for, by the name it would use, and
// what the library calls each one.
var chartTypes = map[string]excelize.ChartType{
	"column":                 excelize.Col,
	"column_stacked":         excelize.ColStacked,
	"column_percent_stacked": excelize.ColPercentStacked,
	"bar":                    excelize.Bar,
	"bar_stacked":            excelize.BarStacked,
	"line":                   excelize.Line,
	"area":                   excelize.Area,
	"area_stacked":           excelize.AreaStacked,
	"pie":                    excelize.Pie,
	"doughnut":               excelize.Doughnut,
	"radar":                  excelize.Radar,
	"scatter":                excelize.Scatter,
}

func (s *session) addChart() protocol.Tool {
	return protocol.Tool{
		Name: "add_chart",
		Description: "Draw a chart from a range of cells and place it on a sheet, beside " +
			"the data it reads. The range's first row is the series names and its first " +
			"column is the category labels unless you say otherwise, so A1:D13 reads as three " +
			"series of twelve points. The chart is linked to the cells: change a figure and " +
			"the chart follows.",
		Schema: protocol.Object(map[string]any{
			"path":           protocol.String("The workbook to change."),
			"sheet":          protocol.String("The sheet to read from and to place the chart on."),
			"source":         protocol.String("The range holding the data, such as A1:D13."),
			"type":           protocol.Choice("The kind of chart.", "column", "column_stacked", "column_percent_stacked", "bar", "bar_stacked", "line", "area", "area_stacked", "pie", "doughnut", "radar", "scatter"),
			"title":          protocol.String("The chart's title."),
			"position":       protocol.String("The cell the chart's top-left corner sits at, such as F2. Put it somewhere empty."),
			"width":          protocol.Integer("The chart's width in pixels, 240 to 1600.", 240, 1600),
			"height":         protocol.Integer("The chart's height in pixels, 160 to 1200.", 160, 1200),
			"has_header":     protocol.Boolean("Read the first row as the series names. On unless set false."),
			"has_categories": protocol.Boolean("Read the first column as the category labels. On unless set false; set false when every column is data."),
			"show_legend":    protocol.Boolean("Show the legend. On unless set false."),
		}, "path", "source", "type"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			source, err := protocol.RequiredString(args, "source", 100)
			if err != nil {
				return "", err
			}
			kind, err := protocol.RequiredChoice(args, "type",
				"column", "column_stacked", "column_percent_stacked", "bar", "bar_stacked",
				"line", "area", "area_stacked", "pie", "doughnut", "radar", "scatter")
			if err != nil {
				return "", err
			}
			title, err := protocol.OptionalString(args, "title", "", 200)
			if err != nil {
				return "", err
			}
			position, err := protocol.OptionalString(args, "position", "", 20)
			if err != nil {
				return "", err
			}
			width, err := protocol.OptionalInt(args, "width", 640, 240, 1600)
			if err != nil {
				return "", err
			}
			height, err := protocol.OptionalInt(args, "height", 420, 160, 1200)
			if err != nil {
				return "", err
			}
			hasHeader, err := protocol.OptionalBool(args, "has_header", true)
			if err != nil {
				return "", err
			}
			hasCategories, err := protocol.OptionalBool(args, "has_categories", true)
			if err != nil {
				return "", err
			}
			showLegend, err := protocol.OptionalBool(args, "show_legend", true)
			if err != nil {
				return "", err
			}

			x1, y1, x2, y2, err := chartCorners(source)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if position == "" {
					// To the right of the data, and a row clear of it, so the
					// chart does not cover what it reads.
					position, err = excelize.CoordinatesToCellName(x2+2, y1)
					if err != nil {
						return "", err
					}
				} else if position, err = workbook.NormalizeCell(position); err != nil {
					return "", err
				}

				chart, err := buildChart(sheet, kind, title, source, x1, y1, x2, y2, hasHeader, hasCategories, showLegend, uint(width), uint(height))
				if err != nil {
					return "", err
				}
				if err := book.File().AddChart(sheet, position, chart); err != nil {
					return "", fmt.Errorf("add the chart to %s: %w", sheet, err)
				}
				return fmt.Sprintf("Added a %s chart on %s at %s, reading %s.", strings.ReplaceAll(kind, "_", " "), sheet, position, source), nil
			})
		},
	}
}

// chartCorners reads a chart's source range, insisting on a block with at least
// a row and a column, because a chart of one cell is a mistake rather than a
// request.
func chartCorners(source string) (x1, y1, x2, y2 int, err error) {
	start, end, err := workbook.ParseRange(source)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	x1, y1, x2, y2, err = workbook.Corners(start, end)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if x1 == x2 && y1 == y2 {
		return 0, 0, 0, 0, fmt.Errorf("a chart needs a range of cells, not the single cell %s", start)
	}
	return x1, y1, x2, y2, nil
}

// buildChart turns a range into series. The range is read as a table: optional
// header row, optional first column of labels, and one series per remaining
// column — which is how a person reads a spreadsheet and what they expect a
// chart to follow.
func buildChart(sheet, kind, title, source string, x1, y1, x2, y2 int, hasHeader, hasCategories, showLegend bool, width, height uint) (*excelize.Chart, error) {
	firstDataRow := y1
	if hasHeader {
		firstDataRow++
	}
	firstSeriesColumn := x1
	if hasCategories {
		firstSeriesColumn++
	}
	if firstSeriesColumn > x2 {
		return nil, fmt.Errorf("the range %s has one column, which is all category labels: "+
			"set has_categories false if it is the data, or give a wider range", source)
	}
	if firstDataRow > y2 {
		return nil, fmt.Errorf("the range %s has no data rows left after the header row: set has_header false, or give a taller range", source)
	}

	var categories string
	if hasCategories {
		first, err := excelize.CoordinatesToCellName(x1, firstDataRow)
		if err != nil {
			return nil, err
		}
		last, err := excelize.CoordinatesToCellName(x1, y2)
		if err != nil {
			return nil, err
		}
		categories = workbook.Absolute(sheet, first, last)
	}

	var series []excelize.ChartSeries
	for column := firstSeriesColumn; column <= x2; column++ {
		valuesFirst, err := excelize.CoordinatesToCellName(column, firstDataRow)
		if err != nil {
			return nil, err
		}
		valuesLast, err := excelize.CoordinatesToCellName(column, y2)
		if err != nil {
			return nil, err
		}
		entry := excelize.ChartSeries{
			Categories: categories,
			Values:     workbook.Absolute(sheet, valuesFirst, valuesLast),
		}
		if hasHeader {
			nameCell, err := excelize.CoordinatesToCellName(column, y1)
			if err != nil {
				return nil, err
			}
			entry.Name = workbook.Absolute(sheet, nameCell, nameCell)
		} else {
			entry.Name = fmt.Sprintf("Series %d", column-firstSeriesColumn+1)
		}
		series = append(series, entry)
	}

	switch kind {
	case "pie", "doughnut":
		// Every slice a pie draws beyond the first is a slice nobody asked for.
		if len(series) > 1 {
			series = series[:1]
		}
	}

	chart := &excelize.Chart{
		Type:   chartTypes[kind],
		Series: series,
		Dimension: excelize.ChartDimension{
			Width:  width,
			Height: height,
		},
		Format: excelize.GraphicOptions{OffsetX: 5, OffsetY: 5},
	}
	if showLegend {
		chart.Legend = excelize.ChartLegend{Position: "bottom"}
	}
	if title != "" {
		chart.Title = excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: title}}}
	}
	return chart, nil
}
