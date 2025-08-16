package notionutils

import (
	"time"

	"github.com/dstotijn/go-notion"
)

func BuildTitleProperty(title string) notion.DatabasePageProperty {
	return notion.DatabasePageProperty{
		Title: []notion.RichText{
			{
				Text: &notion.Text{
					Content: title,
					Link:    nil,
				},
			},
		},
	}
}

func BuildDateProperty(start, end int64, hasTime bool) notion.DatabasePageProperty {
	startDate := notion.NewDateTime(time.Unix(start, 0), hasTime)
	dateProp := notion.DatabasePageProperty{
		Date: &notion.Date{
			Start: startDate,
		},
	}
	if end >= start {
		endDate := notion.NewDateTime(time.Unix(end, 0), hasTime)
		dateProp.Date.End = &endDate
	}
	return dateProp
}

func BuildRichTextProperty(text string) notion.DatabasePageProperty {
	return notion.DatabasePageProperty{
		RichText: []notion.RichText{
			{
				Text: &notion.Text{
					Content: text,
					Link:    nil,
				},
			},
		},
	}
}

func BuildSelectProperty(option string) notion.DatabasePageProperty {
	return notion.DatabasePageProperty{
		Select: &notion.SelectOptions{
			Name: option,
		},
	}
}

func BuildMultiSelectProperty(options []string) notion.DatabasePageProperty {
	multiSelectOptions := make([]notion.SelectOptions, 0, len(options))
	for _, option := range options {
		multiSelectOptions = append(multiSelectOptions, notion.SelectOptions{Name: option})
	}
	return notion.DatabasePageProperty{
		MultiSelect: multiSelectOptions,
	}
}

func BuildNumberProperty(number float64) notion.DatabasePageProperty {
	return notion.DatabasePageProperty{
		Number: &number,
	}
}

func BuildRelationProperty(pagesIDs []string) notion.DatabasePageProperty {
	relations := make([]notion.Relation, 0, len(pagesIDs))

	for _, pageID := range pagesIDs {
		relations = append(relations, notion.Relation{ID: pageID})
	}

	return notion.DatabasePageProperty{
		Relation: relations,
	}
}

func GetTitleValue(dbProperty notion.DatabasePageProperty) string {
	if len(dbProperty.Title) <= 0 {
		return ""
	}
	return dbProperty.Title[0].PlainText
}

func GetDateValue(dbProperty notion.DatabasePageProperty) (*time.Time, *time.Time) {
	var start, end *time.Time
	if dbProperty.Date != nil {
		start = &dbProperty.Date.Start.Time
		if dbProperty.Date.End != nil {
			end = &dbProperty.Date.End.Time
		}
	}
	return start, end
}

func GetRichTextValue(dbProperty notion.DatabasePageProperty) string {
	if len(dbProperty.RichText) <= 0 {
		return ""
	}
	return dbProperty.RichText[0].PlainText
}

func GetSelectValue(dbProperty notion.DatabasePageProperty) string {
	if dbProperty.Select == nil {
		return ""
	}
	return dbProperty.Select.Name
}

func GetMultiSelectValue(dbProperty notion.DatabasePageProperty) []string {
	values := make([]string, 0, len(dbProperty.MultiSelect))
	for _, value := range dbProperty.MultiSelect {
		values = append(values, value.Name)
	}
	return values
}

func GetNumberValue(dbProperty notion.DatabasePageProperty) float64 {
	if dbProperty.Number == nil {
		return 0.0
	}
	return *dbProperty.Number
}

// Queries

func BuildDatePropertyQueryOn(column string, start, end time.Time) notion.DatabaseQueryFilter {
	return notion.DatabaseQueryFilter{
		Property: column,

		DatabaseQueryPropertyFilter: notion.DatabaseQueryPropertyFilter{
			Date: &notion.DatePropertyFilter{
				OnOrAfter:  &start,
				OnOrBefore: &end,
			},
		},
	}
}
