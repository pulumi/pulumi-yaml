// Copyright 2022, Pulumi Corporation.  All rights reserved.

package diags

import (
	"fmt"
	"strings"
)

// A formatter for when a field or property is used that does not exist.
type NonExistantFieldFormatter struct {
	ParentLabel         string
	Fields              []string
	MaxElements         int
	FieldsAreProperties bool
}

func (e NonExistantFieldFormatter) fieldsName() string {
	if e.FieldsAreProperties {
		return "properties"
	}
	return "fields"
}

// Get a single line message.
func (e NonExistantFieldFormatter) Message(field, fieldLabel string) string {
	return fmt.Sprintf("%s %s", e.messageHeader(fieldLabel), e.messageBody(field))
}

// A message broken up into a top level and detail line
func (e NonExistantFieldFormatter) MessageWithDetail(field, fieldLabel string) (string, string) {
	return e.messageHeader(fieldLabel), e.messageBody(field)
}

func (e NonExistantFieldFormatter) messageHeader(fieldLabel string) string {
	return fmt.Sprintf("%s does not exist on %s.", fieldLabel, e.ParentLabel)
}

func (e NonExistantFieldFormatter) messageBody(field string) string {
	existing := sortByEditDistance(e.Fields, field)
	if len(existing) == 0 {
		return fmt.Sprintf("%s has no %s", e.ParentLabel, e.fieldsName())
	}
	list := strings.Join(existing, ", ")
	if len(existing) > e.MaxElements && e.MaxElements != 0 {
		list = fmt.Sprintf("%s and %d others", strings.Join(existing[:5], ", "), len(existing)-e.MaxElements)
	}
	return fmt.Sprintf("Existing %s are: %s", e.fieldsName(), list)
}
