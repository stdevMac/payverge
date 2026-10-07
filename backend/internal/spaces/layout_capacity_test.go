package spaces

import "testing"

// L3-24: publish validation rejects capacity 0 and min > max.
func TestValidatePublish_CapacityInvariants(t *testing.T) {
	zero := 0
	min5 := 5
	max2 := 2
	max4 := 4
	min1 := 1

	doc := &LayoutDocument{
		SchemaVersion: LayoutSchemaVersion,
		WidthMm:       5000,
		HeightMm:      5000,
		Tables: []LayoutTable{
			{
				TableID:     1,
				Name:        "T1",
				XMm:         100,
				YMm:         100,
				WidthMm:     800,
				HeightMm:    800,
				Shape:       "round",
				MaxCapacity: &zero,
			},
		},
	}
	res := ValidateLayoutDocumentMode(doc, ValidatePublish)
	if res.Valid {
		t.Fatalf("expected invalid for max_capacity=0, got valid")
	}

	doc.Tables[0].MaxCapacity = &max4
	doc.Tables[0].MinCapacity = &min5
	res = ValidateLayoutDocumentMode(doc, ValidatePublish)
	if res.Valid {
		t.Fatalf("expected invalid for min_capacity > max_capacity")
	}

	doc.Tables[0].MinCapacity = &min1
	doc.Tables[0].MaxCapacity = &max2
	res = ValidateLayoutDocumentMode(doc, ValidatePublish)
	if !res.Valid {
		t.Fatalf("expected valid capacity bounds, issues=%v", res.Issues)
	}
}

// L3-24 follow-up: the capacity invariant is a PUBLISH gate, not a draft gate.
//
// Hard-failing draft saves strands anyone whose layout already holds an
// out-of-range capacity (rows predate the rule, and the CRUD side never
// clamped): every autosave 400s and the editor becomes unusable with no way to
// reach a state the backend will accept. Draft must persist and surface the
// problem as a warning; publish is where it blocks.
func TestValidateDraft_CapacityWarnsButDoesNotBlock(t *testing.T) {
	huge := 150
	min5 := 5
	max2 := 2
	zero := 0

	newDoc := func() *LayoutDocument {
		return &LayoutDocument{
			SchemaVersion: LayoutSchemaVersion,
			WidthMm:       5000,
			HeightMm:      5000,
			Tables: []LayoutTable{
				{
					TableID:  1,
					Name:     "T1",
					XMm:      100,
					YMm:      100,
					WidthMm:  800,
					HeightMm: 800,
					Shape:    "round",
				},
			},
		}
	}

	hasWarning := func(res ValidationResult) bool {
		for _, w := range res.Warnings {
			if w.Code == "table_capacity" {
				return true
			}
		}
		return false
	}

	// Out-of-range max on an existing draft.
	doc := newDoc()
	doc.Tables[0].MaxCapacity = &huge
	res := ValidateLayoutDocumentMode(doc, ValidateDraft)
	if !res.Valid {
		t.Fatalf("draft save with max_capacity=150 must succeed, issues=%v", res.Issues)
	}
	if !hasWarning(res) {
		t.Fatalf("draft with max_capacity=150 must warn, warnings=%v", res.Warnings)
	}
	if res := ValidateLayoutDocumentMode(doc, ValidatePublish); res.Valid {
		t.Fatalf("publish with max_capacity=150 must fail")
	}

	// Zero capacity on an existing draft.
	doc = newDoc()
	doc.Tables[0].MaxCapacity = &zero
	if res := ValidateLayoutDocumentMode(doc, ValidateDraft); !res.Valid {
		t.Fatalf("draft save with max_capacity=0 must succeed, issues=%v", res.Issues)
	}

	// min > max on an existing draft.
	doc = newDoc()
	doc.Tables[0].MinCapacity = &min5
	doc.Tables[0].MaxCapacity = &max2
	res = ValidateLayoutDocumentMode(doc, ValidateDraft)
	if !res.Valid {
		t.Fatalf("draft save with min>max must succeed, issues=%v", res.Issues)
	}
	if !hasWarning(res) {
		t.Fatalf("draft with min>max must warn, warnings=%v", res.Warnings)
	}
	if res := ValidateLayoutDocumentMode(doc, ValidatePublish); res.Valid {
		t.Fatalf("publish with min>max must fail")
	}
}
