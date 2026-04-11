package graph

import (
	"fmt"
	"sort"
	"strings"
)

// ClassMember represents a method, attribute, or type in a class.
type ClassMember struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`       // method, attribute, type, event
	Visibility string `json:"visibility"` // public, protected, private
	Level      string `json:"level"`      // instance, static
	ADTType    string `json:"adt_type"`   // CLAS/OM, CLAS/OA, CLAS/OT, CLAS/OO
}

// ClassSection groups members by visibility.
type ClassSection struct {
	Visibility string        `json:"visibility"` // PUBLIC, PROTECTED, PRIVATE
	Methods    []ClassMember `json:"methods,omitempty"`
	Attributes []ClassMember `json:"attributes,omitempty"`
	Types      []ClassMember `json:"types,omitempty"`
	Events     []ClassMember `json:"events,omitempty"`
}

// ClassSectionsResult is the output of ClassifySections.
type ClassSectionsResult struct {
	ClassName  string                  `json:"class_name"`
	Sections   []ClassSection          `json:"sections"`
	Summary    ClassSectionsSummary    `json:"summary"`
}

// ClassSectionsSummary provides counts for quick overview.
type ClassSectionsSummary struct {
	TotalMembers int            `json:"total_members"`
	ByVisibility map[string]int `json:"by_visibility"` // PUBLIC: 5, PROTECTED: 2, PRIVATE: 8
	ByKind       map[string]int `json:"by_kind"`       // method: 10, attribute: 3, type: 2
}

// ClassStructureElement is the input: one element from ADT objectstructure.
// Maps to ClassObjectStructureElement fields.
type ClassStructureElement struct {
	Name       string // METHOD_NAME, MV_ATTRIBUTE, etc.
	ADTType    string // CLAS/OM (method), CLAS/OA (attribute), CLAS/OT (type), CLAS/OO (event)
	Visibility string // public, protected, private
	Level      string // instance, static
}

// ClassifySections groups class structure elements into PUBLIC/PROTECTED/PRIVATE sections.
func ClassifySections(className string, elements []ClassStructureElement) *ClassSectionsResult {
	result := &ClassSectionsResult{
		ClassName: strings.ToUpper(strings.TrimSpace(className)),
		Summary: ClassSectionsSummary{
			ByVisibility: make(map[string]int),
			ByKind:       make(map[string]int),
		},
	}

	// Group by visibility
	sectionMap := map[string]*ClassSection{
		"public":    {Visibility: "PUBLIC"},
		"protected": {Visibility: "PROTECTED"},
		"private":   {Visibility: "PRIVATE"},
	}

	for _, elem := range elements {
		vis := strings.ToLower(strings.TrimSpace(elem.Visibility))
		if vis == "" {
			vis = "private" // default to private if unknown
		}

		section, ok := sectionMap[vis]
		if !ok {
			section = sectionMap["private"]
			vis = "private"
		}

		kind := classifyMemberKind(elem.ADTType)

		member := ClassMember{
			Name:       elem.Name,
			Kind:       kind,
			Visibility: strings.ToUpper(vis),
			Level:      elem.Level,
			ADTType:    elem.ADTType,
		}

		switch kind {
		case "method":
			section.Methods = append(section.Methods, member)
		case "attribute":
			section.Attributes = append(section.Attributes, member)
		case "type":
			section.Types = append(section.Types, member)
		case "event":
			section.Events = append(section.Events, member)
		default:
			section.Attributes = append(section.Attributes, member) // fallback
		}

		result.Summary.TotalMembers++
		result.Summary.ByVisibility[strings.ToUpper(vis)]++
		result.Summary.ByKind[kind]++
	}

	// Build ordered sections: PUBLIC → PROTECTED → PRIVATE
	for _, vis := range []string{"public", "protected", "private"} {
		section := sectionMap[vis]
		if len(section.Methods)+len(section.Attributes)+len(section.Types)+len(section.Events) > 0 {
			// Sort members within each group
			sort.Slice(section.Methods, func(i, j int) bool { return section.Methods[i].Name < section.Methods[j].Name })
			sort.Slice(section.Attributes, func(i, j int) bool { return section.Attributes[i].Name < section.Attributes[j].Name })
			sort.Slice(section.Types, func(i, j int) bool { return section.Types[i].Name < section.Types[j].Name })
			sort.Slice(section.Events, func(i, j int) bool { return section.Events[i].Name < section.Events[j].Name })
			result.Sections = append(result.Sections, *section)
		}
	}

	return result
}

// FormatClassSections renders a human-readable text view.
func FormatClassSections(r *ClassSectionsResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Class: %s (%d members)\n\n", r.ClassName, r.Summary.TotalMembers))

	for _, section := range r.Sections {
		sb.WriteString(fmt.Sprintf("=== %s ===\n", section.Visibility))

		if len(section.Methods) > 0 {
			sb.WriteString("  Methods:\n")
			for _, m := range section.Methods {
				level := ""
				if m.Level == "static" {
					level = " [static]"
				}
				sb.WriteString(fmt.Sprintf("    %s%s\n", m.Name, level))
			}
		}
		if len(section.Attributes) > 0 {
			sb.WriteString("  Attributes:\n")
			for _, a := range section.Attributes {
				level := ""
				if a.Level == "static" {
					level = " [static]"
				}
				sb.WriteString(fmt.Sprintf("    %s%s\n", a.Name, level))
			}
		}
		if len(section.Types) > 0 {
			sb.WriteString("  Types:\n")
			for _, t := range section.Types {
				sb.WriteString(fmt.Sprintf("    %s\n", t.Name))
			}
		}
		if len(section.Events) > 0 {
			sb.WriteString("  Events:\n")
			for _, e := range section.Events {
				sb.WriteString(fmt.Sprintf("    %s\n", e.Name))
			}
		}
		sb.WriteByte('\n')
	}

	// Summary
	sb.WriteString("Summary:")
	for _, vis := range []string{"PUBLIC", "PROTECTED", "PRIVATE"} {
		if count, ok := r.Summary.ByVisibility[vis]; ok {
			sb.WriteString(fmt.Sprintf(" %s=%d", vis, count))
		}
	}
	sb.WriteByte('\n')

	return sb.String()
}

// classifyMemberKind maps ADT type codes to member kinds.
func classifyMemberKind(adtType string) string {
	switch adtType {
	case "CLAS/OM":
		return "method"
	case "CLAS/OA":
		return "attribute"
	case "CLAS/OT":
		return "type"
	case "CLAS/OO":
		return "event"
	default:
		return "unknown"
	}
}
