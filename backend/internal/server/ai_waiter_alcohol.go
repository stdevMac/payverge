package server

import "strings"

// Issue 941: a combined kids / vegetarian / pregnancy ask was answered with
// "Ensalada de estación" paired with a "Copa de Malbec". The deterministic
// finalizer had no alcohol awareness whatsoever — the companion is simply the
// first candidate from another snapshot category, and a wine carries the
// vegetarian and vegan tags, so it survived every existing filter.
//
// The menu model has no "is alcoholic" column, so this reads the only signal
// the snapshot actually carries: the words on the card. That is a heuristic,
// and it is deliberately biased toward over-detecting alcohol — dropping a
// borderline drink from a pregnancy or kids answer is a far cheaper error than
// pouring wine into one.

// waiterAlcoholFreeMarkers are whole-token phrases: a pregnancy, a table with
// children, or an explicit alcohol-free request.
var waiterAlcoholFreeMarkers = []string{
	// pregnancy / nursing
	"embarazada", "embarazadas", "embarazo", "encinta", "amamantando", "lactancia",
	"pregnant", "pregnancy", "expecting", "breastfeeding", "nursing",
	"enceinte", "schwanger", "incinta", "gravida", "grávida", "zwanger", "gravid",
	// children at the table
	"niño", "niños", "nino", "ninos", "nena", "nenas", "nene", "nenes", "chico", "chicos",
	"hijo", "hija", "hijos", "hijas", "menores", "infantil", "infantiles",
	"kid", "kids", "child", "children", "toddler", "toddlers",
	"enfant", "enfants", "kinder", "bambini", "criança", "crianças", "kinderen", "barn",
	// explicit
	"sin alcohol", "no alcohol", "non alcoholic", "nonalcoholic", "alcohol free",
	"sin bebidas alcoholicas", "sin bebidas alcohólicas", "abstemio", "abstemia",
	"soy el conductor designado", "designated driver",
}

// waiterAlcoholFreeSubstringMarkers cover scripts written without spaces, where
// whole-token matching cannot work.
var waiterAlcoholFreeSubstringMarkers = []string{
	"妊娠", "임신", "ตั้งครรภ์", "怀孕", "мang thai", "беремен",
	"子供", "아이", "เด็ก", "孩子", "детей", "ребен",
	"ノンアルコール", "무알콜", "ไม่มีแอลกอฮอล์", "无酒精", "без алкоголя",
}

// waiterAlcoholWords are the drink words that make an entity alcoholic. Matched
// as whole tokens so "vino" cannot fire inside "vinagre".
var waiterAlcoholWords = []string{
	"vino", "vinos", "wine", "wines", "vin", "wein", "vinho",
	"malbec", "cabernet", "merlot", "syrah", "shiraz", "tempranillo", "bonarda", "torrontes", "torrontés",
	"chardonnay", "sauvignon", "pinot", "rioja", "prosecco", "champagne", "champán", "champan",
	"espumante", "espumantes", "sparkling wine", "tinto", "rosado", "rosé",
	"cerveza", "cervezas", "beer", "beers", "ipa", "lager", "stout", "pilsen", "pilsener", "bier", "birra",
	"whisky", "whiskey", "bourbon", "vodka", "gin", "ginebra", "ron", "rum", "tequila", "mezcal", "pisco",
	"aperitivo", "aperitivos", "aperol", "campari", "fernet", "vermut", "vermouth", "vermú",
	"licor", "licores", "liqueur", "liquor", "spirits", "sake", "soju", "grappa",
	"coctel", "cóctel", "cocteles", "cócteles", "cocktail", "cocktails", "tragos", "trago",
	"caipirinha", "mojito", "margarita", "negroni", "spritz", "sangria", "sangría", "clerico", "clericó",
	"sidra", "cider", "hidromiel", "mead",
	"copa de vino", "carta de vinos", "bar",
}

// waiterCategoryNamesByID indexes the snapshot's category display names so an
// entity can be judged by the section it sits in ("Vinos", "Cócteles").
func waiterCategoryNamesByID(snapshot WaiterMenuSnapshot) map[string]string {
	names := make(map[string]string, len(snapshot.Categories))
	for _, category := range snapshot.Categories {
		names[category.ID] = category.DisplayName
	}
	return names
}

// waiterAlcoholFreeAsk reports whether the guest's message rules out alcohol.
func waiterAlcoholFreeAsk(userMessage string) bool {
	if strings.TrimSpace(userMessage) == "" {
		return false
	}
	if containsAnyWaiterPhrase(userMessage, waiterAlcoholFreeMarkers) {
		return true
	}
	lower := strings.ToLower(userMessage)
	for _, marker := range waiterAlcoholFreeSubstringMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// waiterEntityIsAlcoholic reads the words on the card — both names, the
// description, and the section title — for a drink word.
func waiterEntityIsAlcoholic(entity WaiterMenuEntity, categoryNames map[string]string) bool {
	haystack := strings.Join([]string{
		entity.DisplayName, entity.SourceName, entity.Description, categoryNames[entity.CategoryID],
	}, " ")
	return containsAnyWaiterPhrase(haystack, waiterAlcoholWords)
}

// filterWaiterAlcoholicEntities drops every alcoholic candidate, preserving order.
func filterWaiterAlcoholicEntities(snapshot WaiterMenuSnapshot, entities []WaiterMenuEntity) []WaiterMenuEntity {
	categoryNames := waiterCategoryNamesByID(snapshot)
	kept := make([]WaiterMenuEntity, 0, len(entities))
	for _, entity := range entities {
		if waiterEntityIsAlcoholic(entity, categoryNames) {
			continue
		}
		kept = append(kept, entity)
	}
	return kept
}

// waiterRecommendableEntitiesForAsk fetches recommendable snapshot entities for
// one guest ask, dropping alcohol *before* the top-N truncation when the ask
// rules it out.
//
// Order matters. RecommendableEntities walks the snapshot in carta order, so on
// a wine-forward carta the first N recommendables are all wine; truncating to N
// first and filtering afterwards leaves an empty slice, and the finalizer falls
// through to "no encontré eso en el menú". A pregnant guest asking for a
// vegetarian dish was told the menu had nothing — strictly worse than the
// pre-941 behaviour, where she at least got food beside the wine.
func waiterRecommendableEntitiesForAsk(
	snapshot WaiterMenuSnapshot, dietaryTags []string, limit int, alcoholFree bool,
) []WaiterMenuEntity {
	if !alcoholFree {
		return snapshot.RecommendableEntities(dietaryTags, limit)
	}
	kept := filterWaiterAlcoholicEntities(snapshot, snapshot.RecommendableEntities(dietaryTags, 0))
	if limit > 0 && len(kept) > limit {
		kept = kept[:limit]
	}
	return kept
}
