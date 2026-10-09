package database

import "testing"

// TestCanonicalizeConstraintDef_CollapsesBenignCastSpelling proves the two
// equivalent Postgres deparse spellings of the same varchar-column CHECK — the
// "authored via IN(...)" array-cast form and the "restored from pg_dump"
// per-element form — canonicalize to one identical string, while genuinely
// different predicates stay distinct. This is the Docker-free guard that the
// fpConstraints normalizer neither over- nor under-collapses.
func TestCanonicalizeConstraintDef_CollapsesBenignCastSpelling(t *testing.T) {
	// The same varchar-column CHECK, three equivalent Postgres serializations:
	//  - restored: per-element ::character varying::text (genesis bootstrap catalog)
	//  - bare:     ARRAY[...]::text[]   (reconciled catalog, pg_get_constraintdef pretty=true)
	//  - paren:    (ARRAY[...])::text[] (pg_dump baseline text)
	restored := `cash_register_movements.cash_register_movements_type_check|CHECK (movement_type = ANY (ARRAY['cash_sale'::character varying::text, 'cash_refund'::character varying::text, 'cash_in'::character varying::text, 'cash_out'::character varying::text]))`
	bare := `cash_register_movements.cash_register_movements_type_check|CHECK (movement_type = ANY (ARRAY['cash_sale'::character varying, 'cash_refund'::character varying, 'cash_in'::character varying, 'cash_out'::character varying]::text[]))`
	paren := `cash_register_movements.cash_register_movements_type_check|CHECK (movement_type = ANY ((ARRAY['cash_sale'::character varying, 'cash_refund'::character varying, 'cash_in'::character varying, 'cash_out'::character varying])::text[]))`

	cRestored := canonicalizeConstraintDef(restored)
	cBare := canonicalizeConstraintDef(bare)
	cParen := canonicalizeConstraintDef(paren)
	if cRestored != cBare || cBare != cParen {
		t.Fatalf("all three spellings must canonicalize equal:\n restored -> %s\n bare     -> %s\n paren    -> %s", cRestored, cBare, cParen)
	}

	// The <> ALL manual-reason form (restored vs bare) must also collapse.
	restoredReason := `x.y|CHECK ((movement_type <> ALL (ARRAY['cash_in'::character varying::text, 'cash_out'::character varying::text])) OR length(TRIM(BOTH FROM reason)) > 0)`
	bareReason := `x.y|CHECK ((movement_type <> ALL (ARRAY['cash_in'::character varying, 'cash_out'::character varying]::text[])) OR length(TRIM(BOTH FROM reason)) > 0)`
	if canonicalizeConstraintDef(restoredReason) != canonicalizeConstraintDef(bareReason) {
		t.Fatalf("manual-reason CHECK spellings must canonicalize equal:\n %s\n %s",
			canonicalizeConstraintDef(restoredReason), canonicalizeConstraintDef(bareReason))
	}

	// A real predicate change (different allowed value) must NOT collapse — the
	// normalizer only erases cast spelling, never predicate substance.
	changed := `cash_register_movements.cash_register_movements_type_check|CHECK (movement_type = ANY (ARRAY['cash_sale'::character varying, 'cash_refund'::character varying, 'cash_in'::character varying, 'cash_frozen'::character varying]::text[]))`
	if canonicalizeConstraintDef(bare) == canonicalizeConstraintDef(changed) {
		t.Fatal("a changed allowed value must NOT canonicalize equal to the original")
	}

	// A different operator (ANY vs ALL) must NOT collapse.
	anyForm := `t.c|CHECK (col = ANY (ARRAY['a'::character varying]::text[]))`
	allForm := `t.c|CHECK (col = ALL (ARRAY['a'::character varying]::text[]))`
	if canonicalizeConstraintDef(anyForm) == canonicalizeConstraintDef(allForm) {
		t.Fatal("ANY vs ALL must NOT canonicalize equal")
	}
}
