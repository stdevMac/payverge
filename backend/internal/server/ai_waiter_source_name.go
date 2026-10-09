package server

// Issue 869: on a translated menu the waiter answered with the localized dish
// name ("Ribeye") while the guest card the guest is holding still reads the
// stored name ("Ojo de bife"), so guest and waiter were not talking about the
// same dish. Prose therefore names a translated dish BOTH ways —
// "Ribeye (Ojo de bife)" — while canonical identity (entity display names,
// source titles, action targets, category titles) stays the single localized
// name so nothing downstream has to parse a parenthetical.
//
// Both halves are sanitized separately and only then joined, because
// safeWaiterPresentation strips parentheses: sanitizing the joined string would
// erase the very brackets this renders.

// waiterEntityProseSuffix renders the untranslated menu name as a parenthetical.
// Empty when the menu is not translated (the names agree), when either name is
// missing, or when the pair would not fit the presentation budget.
func waiterEntityProseSuffix(entity WaiterMenuEntity) string {
	display := safeWaiterPresentation(entity.DisplayName)
	source := safeWaiterPresentation(entity.SourceName)
	if display == "" || source == "" {
		return ""
	}
	if normalizeName(display) == normalizeName(source) {
		return ""
	}
	if len([]rune(display))+len([]rune(source))+len(" ()") > maxWaiterPresentationRunes {
		return ""
	}
	return " (" + source + ")"
}

// waiterEntityProseName is the plain-text name for prose: "Ribeye (Ojo de bife)".
func waiterEntityProseName(entity WaiterMenuEntity) string {
	display := safeWaiterPresentation(entity.DisplayName)
	if display == "" {
		display = safeWaiterPresentation(entity.ID)
	}
	return display + waiterEntityProseSuffix(entity)
}

// waiterEntityBoldName emphasizes only the localized name the answer is about,
// keeping the card name legible beside it: "**Ribeye** (Ojo de bife)".
func waiterEntityBoldName(entity WaiterMenuEntity) string {
	display := safeWaiterPresentation(entity.DisplayName)
	if display == "" {
		display = safeWaiterPresentation(entity.ID)
	}
	return "**" + display + "**" + waiterEntityProseSuffix(entity)
}
