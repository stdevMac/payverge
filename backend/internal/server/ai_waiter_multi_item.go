package server

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// waiterMatchedNamePhrase returns the longest snapshot-owned name or alias of
// entity that literally appears in the guest turn. It is the strength of the
// match: a full display name beats the single-token alias the snapshot indexes
// for translated menus.
func waiterMatchedNamePhrase(snapshot WaiterMenuSnapshot, padded string, entity WaiterMenuEntity) string {
	best := ""
	consider := func(name string) {
		for _, phrase := range waiterEntityMatchPhrases(name) {
			if !paddedTextContainsWaiterPhrase(padded, phrase) {
				continue
			}
			if len(phrase) > len(best) {
				best = phrase
			}
		}
	}
	consider(entity.DisplayName)
	for _, alias := range snapshot.sourceNamesByKey[WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}] {
		consider(alias)
	}
	return best
}

// waiterDistinctlyNamedEntities drops a match whose matched name is only a
// fragment of a longer dish name the guest actually typed. "the mixed grill"
// matches Mixed salad through its one-token "mixed" alias, and quoting a salad
// the guest never asked about is a grounding failure once several dishes are
// answered at once (issue 868). Entities that match the same phrase are all
// kept: "the malbec" really is ambiguous between the glass and the bottle, and
// pricing both is the honest answer.
func waiterDistinctlyNamedEntities(snapshot WaiterMenuSnapshot, userMessage string, matched []WaiterMenuEntity) []WaiterMenuEntity {
	if len(matched) < 2 {
		return matched
	}
	padded := waiterPaddedSearchText(userMessage)
	phrases := make([]string, len(matched))
	for i, entity := range matched {
		phrases[i] = waiterMatchedNamePhrase(snapshot, padded, entity)
	}
	kept := make([]WaiterMenuEntity, 0, len(matched))
	for i, entity := range matched {
		if phrases[i] != "" && waiterPhraseIsFragmentOfAnother(phrases, i) {
			continue
		}
		kept = append(kept, entity)
	}
	if len(kept) == 0 {
		return matched
	}
	return kept
}

// waiterEntitiesShareADisplayName reports whether two of the named dishes carry
// the same name. Issue 868 prices every dish a guest can tell apart; two rows
// that normalize to one name are not two dishes to the guest, they are one name
// with two prices, and printing it twice answers nothing they can act on. The
// cart validator already refuses such a name for the same reason, so the turn
// asks which one instead of pretending to have answered.
func waiterEntitiesShareADisplayName(entities []WaiterMenuEntity) bool {
	if len(entities) < 2 {
		return false
	}
	seen := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		name := normalizeWaiterSearchText(entity.DisplayName)
		if name == "" {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			return true
		}
		seen[name] = struct{}{}
	}
	return false
}

func waiterPhraseIsFragmentOfAnother(phrases []string, index int) bool {
	phrase := " " + phrases[index] + " "
	for i, other := range phrases {
		if i == index || other == "" || len(other) <= len(phrases[index]) {
			continue
		}
		if strings.Contains(" "+other+" ", phrase) {
			return true
		}
	}
	return false
}

// finalizeWaiterNamedItems answers a turn naming several dishes at once with one
// grounded priced line per dish. Before issue 868 every name past the first
// turned the whole answer into "I couldn't find that on the menu", even though
// the snapshot had already resolved each one.
func finalizeWaiterNamedItems(
	response *assistantcontract.Response, locale string,
	entities []WaiterMenuEntity, copy waiterV2Copy, retrievedAt string,
) {
	truncated := false
	if len(entities) > maxWaiterMenuSources {
		entities = entities[:maxWaiterMenuSources]
		truncated = true
	}
	presented := make([]WaiterMenuEntity, 0, len(entities))
	for _, entity := range entities {
		entityID, sourceID := appendWaiterEntityAndSource(response, entity, retrievedAt)
		if entityID == "" || sourceID == "" {
			truncated = true
			continue
		}
		presented = append(presented, entity)
	}
	if len(presented) == 0 {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	response.Answer.Content = waiterEntityBulletList(presented, copy)
	if truncated {
		response.Status = assistantcontract.StatusDegraded
	}
}
