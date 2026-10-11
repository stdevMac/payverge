package services

import (
	"strings"
	"unicode"
)

// Waiter discovery intents: the deterministic finalizer needs to know when a
// guest is asking "what's good?", "what goes with X?", "what can I have
// instead?" or "what is vegetarian?" so it can answer from the canonical menu
// snapshot instead of falling through to generic copy.
//
// Detection mirrors DetectAllergenIntent: broad, lowercase substring matching
// against the locale family vocabulary, falling back to English. A false
// positive only routes the turn to a server-grounded menu answer built from the
// snapshot — it can never invent an item, a fact, or a transaction.

var recommendationKeywords = map[string][]string{
	"en":    {"recommend", "suggest", "what should i", "what's good", "whats good", "what is good", "what do you have that is good", "your favorite", "your favourite", "best dish", "most popular", "signature", "what's available", "whats available", "what is available", "actually available", "available right now", "what do you have available"},
	"es":    {"recomi", "recomen", "sugier", "suger", "que me conviene", "qué me conviene", "que esta bueno", "qué está bueno", "tu favorito", "el mejor plato", "mas pedido", "más pedido", "que pido", "qué pido", "especialidad", "que hay disponible", "qué hay disponible", "que esta disponible", "qué está disponible", "disponible ahora"},
	"es_ar": {"recomi", "recomen", "sugier", "suger", "que me conviene", "qué me conviene", "que esta bueno", "qué está bueno", "tu favorito", "el mejor plato", "mas pedido", "más pedido", "que pido", "qué pido", "especialidad", "que hay disponible", "qué hay disponible", "que esta disponible", "qué está disponible", "disponible ahora"},
	"fr":    {"conseill", "recommand", "suggér", "sugger", "suggestion", "qu'est-ce qui est bon", "votre préféré", "votre prefere", "meilleur plat", "spécialité", "specialite"},
	"de":    {"empfehl", "vorschlag", "was ist gut", "was schmeckt", "lieblings", "bestes gericht", "spezialität", "spezialitat"},
	"it":    {"consigli", "suggeri", "cosa è buono", "cosa e buono", "preferito", "piatto migliore", "specialità", "specialita"},
	"pt":    {"recomend", "sugest", "sugir", "o que é bom", "o que e bom", "favorito", "melhor prato", "especialidade"},
	"nl":    {"aanbevel", "aanrad", "raad je aan", "wat is lekker", "favoriet", "beste gerecht", "specialiteit", "suggestie"},
	"da":    {"anbefal", "forslag", "hvad er godt", "favorit", "bedste ret", "specialitet"},
	"no":    {"anbefal", "forslag", "hva er godt", "favoritt", "beste rett", "spesialitet"},
	"sv":    {"rekommend", "förslag", "forslag", "vad är gott", "vad ar gott", "favorit", "bästa rätt", "basta ratt", "specialitet"},
	"pl":    {"polec", "sugesti", "co jest dobre", "ulubione", "najlepsze danie", "specjalność", "specjalnosc"},
	"ru":    {"посовет", "рекоменд", "что вкусн", "любим", "лучшее блюдо", "фирменн"},
	"tr":    {"öner", "oner", "tavsiye", "ne iyi", "favori", "en iyi yemek", "spesiyal"},
	"ar":    {"توصي", "تنصح", "اقترح", "ما هو الجيد", "المفضل", "أفضل طبق", "طبق مميز"},
	"hi":    {"सुझाव", "सिफारिश", "क्या अच्छा", "पसंदीदा", "सबसे अच्छा", "खासियत"},
	"ja":    {"おすすめ", "オススメ", "お勧め", "推奨", "何がおいしい", "何が美味しい", "名物", "人気"},
	"ko":    {"추천", "뭐가 맛있", "인기", "대표 메뉴"},
	"th":    {"แนะนำ", "อะไรอร่อย", "ยอดนิยม", "เมนูเด็ด"},
	"vi":    {"gợi ý", "đề xuất", "món nào ngon", "yêu thích", "ngon nhất", "đặc biệt"},
	"zh":    {"推荐", "推薦", "有什么好吃", "有什麼好吃", "招牌", "人气", "人氣"},
}

var pairingKeywords = map[string][]string{
	"en":    {"pair", "goes with", "go with", "goes well", "along with", "match with", "complement", "side with", "to drink with", "have with", "eat with"},
	"es":    {"marida", "combina", "acompañ", "acompan", "va bien con", "que va con", "qué va con", "para acompañar", "junto con"},
	"es_ar": {"marida", "combina", "acompañ", "acompan", "va bien con", "que va con", "qué va con", "para acompañar", "junto con", "le va bien"},
	"fr":    {"accompagn", "avec quoi", "va avec", "va bien avec", "se marie", "vous avec", "à côté de", "que boire avec"},
	"de":    {"dazu", "passt zu", "was passt", "beilage", "zusammen mit"},
	"it":    {"abbina", "va bene con", "contorno", "insieme a", "da bere con"},
	"pt":    {"combina", "acompanh", "vai bem com", "junto com"},
	"nl":    {"past bij", "erbij", "combineer", "bijgerecht", "samen met"},
	"da":    {"passer til", "sammen med", "tilbehør"},
	"no":    {"passer til", "sammen med", "tilbehør"},
	"sv":    {"passar till", "tillsammans med", "tillbehör"},
	"pl":    {"pasuje do", "razem z", "dodatek do"},
	"ru":    {"подойдет к", "подойдёт к", "сочетается", "вместе с", "гарнир"},
	"tr":    {"yanında", "ile birlikte", "eşlik", "garnitür"},
	"ar":    {"يناسب", "طبق جانبي", "مع طبق", "يتماشى"},
	"hi":    {"के साथ", "साथ में"},
	"ja":    {"に合う", "と一緒", "合わせ", "付け合わせ"},
	"ko":    {"어울리", "곁들", "함께 먹"},
	"th":    {"เข้ากับ", "คู่กับ", "ทานคู่"},
	"vi":    {"kèm với", "đi với", "hợp với", "ăn kèm"},
	"zh":    {"搭配", "配什么", "配什麼", "一起吃", "一起喝"},
}

var alternativeKeywords = map[string][]string{
	"en":    {"alternative", "instead", "substitute", "something else", "another option", "other option", "replace", "similar", "swap", "except", "other than"},
	"es":    {"alternativa", "en su lugar", "en vez", "otra opcion", "otra opción", "otro plato", "reemplaz", "sustitut", "parecido", "similar", "no sea", "que no sea"},
	"es_ar": {"alternativa", "en su lugar", "en vez", "otra opcion", "otra opción", "otro plato", "reemplaz", "sustitut", "parecido", "similar", "no sea", "que no sea"},
	"fr":    {"alternative", "à la place", "a la place", "autre option", "autre plat", "remplac", "similaire", "au lieu"},
	"de":    {"alternative", "stattdessen", "andere option", "anderes gericht", "ersatz", "ähnlich", "ahnlich"},
	"it":    {"alternativa", "invece", "altra opzione", "altro piatto", "sostitu", "simile"},
	"pt":    {"alternativa", "em vez", "outra opção", "outra opcao", "outro prato", "substitu", "parecido", "similar"},
	"nl":    {"alternatief", "in plaats", "andere optie", "ander gerecht", "vervang", "vergelijkbaar"},
	"da":    {"alternativ", "i stedet", "anden mulighed", "erstat", "lignende"},
	"no":    {"alternativ", "i stedet", "annet alternativ", "erstat", "lignende"},
	"sv":    {"alternativ", "istället", "i stället", "ersätt", "liknande"},
	"pl":    {"alternatyw", "zamiast", "inna opcja", "inne danie", "zastęp", "podobn"},
	"ru":    {"альтернатив", "вместо", "другой вариант", "другое блюдо", "замен", "похож"},
	"tr":    {"alternatif", "yerine", "başka seçenek", "baska secenek", "benzer"},
	"ar":    {"بديل", "بدلا", "بدلاً", "خيار آخر", "مشابه"},
	"hi":    {"विकल्प", "बजाय", "दूसरा", "समान"},
	"ja":    {"代わり", "代替", "他の", "似た"},
	"ko":    {"대신", "대체", "다른 메뉴", "비슷"},
	"th":    {"แทน", "ทางเลือก", "อย่างอื่น", "คล้าย"},
	"vi":    {"thay thế", "thay vì", "lựa chọn khác", "món khác", "tương tự"},
	"zh":    {"替代", "代替", "其他", "别的", "類似", "类似"},
}

// dietaryKeywords maps a locale family to canonical dietary tag IDs (the
// frontend DIETARY_TAGS vocabulary) and the phrases that request them.
var dietaryKeywords = map[string]map[string][]string{
	"en": {
		"vegetarian":  {"vegetarian", "veggie"},
		"vegan":       {"vegan", "plant based", "plant-based"},
		"gluten-free": {"gluten free", "gluten-free", "glutenfree", "celiac", "coeliac", "no gluten", "without gluten"},
		"dairy-free":  {"dairy free", "dairy-free", "lactose free", "lactose-free", "no dairy", "without dairy"},
		"nut-free":    {"nut free", "nut-free", "no nuts", "without nuts"},
	},
	"es": {
		"vegetarian":  {"vegetarian"},
		"vegan":       {"vegan", "basado en plantas"},
		"gluten-free": {"sin gluten", "sin tacc", "celiac", "celíac", "celiaqu", "libre de gluten", "no tenga gluten"},
		"dairy-free":  {"sin lácteo", "sin lacteo", "sin lactosa", "sin leche"},
		"nut-free":    {"sin frutos secos", "sin nueces", "sin maní", "sin mani"},
	},
	"es_ar": {
		"vegetarian":  {"vegetarian"},
		"vegan":       {"vegan", "basado en plantas"},
		"gluten-free": {"sin gluten", "sin tacc", "celiac", "celíac", "celiaqu", "libre de gluten", "no tenga gluten"},
		"dairy-free":  {"sin lácteo", "sin lacteo", "sin lactosa", "sin leche"},
		"nut-free":    {"sin frutos secos", "sin nueces", "sin maní", "sin mani"},
	},
	"fr": {
		"vegetarian":  {"végétarien", "vegetarien"},
		"vegan":       {"végan", "vegan", "végétalien", "vegetalien"},
		"gluten-free": {"sans gluten"},
		"dairy-free":  {"sans lactose", "sans produits laitiers"},
		"nut-free":    {"sans fruits à coque", "sans noix"},
	},
	"de": {
		"vegetarian":  {"vegetarisch"},
		"vegan":       {"vegan"},
		"gluten-free": {"glutenfrei", "ohne gluten"},
		"dairy-free":  {"laktosefrei", "ohne milch", "milchfrei"},
		"nut-free":    {"nussfrei", "ohne nüsse", "ohne nusse"},
	},
	"it": {
		"vegetarian":  {"vegetarian"},
		"vegan":       {"vegan"},
		"gluten-free": {"senza glutine"},
		"dairy-free":  {"senza lattosio", "senza latticini"},
		"nut-free":    {"senza frutta a guscio", "senza noci"},
	},
	"pt": {
		"vegetarian":  {"vegetarian"},
		"vegan":       {"vegan"},
		"gluten-free": {"sem glúten", "sem gluten"},
		"dairy-free":  {"sem lactose", "sem laticínios", "sem laticinios"},
		"nut-free":    {"sem nozes", "sem amendoim"},
	},
	"nl": {
		"vegetarian":  {"vegetarisch"},
		"vegan":       {"veganistisch", "vegan"},
		"gluten-free": {"glutenvrij"},
		"dairy-free":  {"lactosevrij", "zuivelvrij"},
		"nut-free":    {"notenvrij"},
	},
	"da": {
		"vegetarian":  {"vegetar"},
		"vegan":       {"vegan"},
		"gluten-free": {"glutenfri"},
		"dairy-free":  {"laktosefri", "mælkefri"},
		"nut-free":    {"nøddefri"},
	},
	"no": {
		"vegetarian":  {"vegetar"},
		"vegan":       {"vegan"},
		"gluten-free": {"glutenfri"},
		"dairy-free":  {"laktosefri", "melkefri"},
		"nut-free":    {"nøttefri"},
	},
	"sv": {
		"vegetarian":  {"vegetarisk"},
		"vegan":       {"vegan"},
		"gluten-free": {"glutenfri"},
		"dairy-free":  {"laktosfri", "mjölkfri"},
		"nut-free":    {"nötfri"},
	},
	"pl": {
		"vegetarian":  {"wegetari"},
		"vegan":       {"wegań", "wegan"},
		"gluten-free": {"bezglutenow", "bez glutenu"},
		"dairy-free":  {"bez laktozy", "bez nabiału", "bez nabialu"},
		"nut-free":    {"bez orzech"},
	},
	"ru": {
		"vegetarian":  {"вегетариан"},
		"vegan":       {"веган"},
		"gluten-free": {"без глютена"},
		"dairy-free":  {"без лактозы", "без молок"},
		"nut-free":    {"без орех"},
	},
	"tr": {
		"vegetarian":  {"vejetaryen"},
		"vegan":       {"vegan"},
		"gluten-free": {"glütensiz", "glutensiz"},
		"dairy-free":  {"laktozsuz", "sütsüz"},
		"nut-free":    {"fındıksız", "kuruyemişsiz"},
	},
	"ar": {
		"vegetarian":  {"نباتي"},
		"vegan":       {"نباتي صرف", "فيغان"},
		"gluten-free": {"خال من الغلوتين", "بدون غلوتين"},
		"dairy-free":  {"خال من الألبان", "بدون ألبان"},
		"nut-free":    {"خال من المكسرات", "بدون مكسرات"},
	},
	"hi": {
		"vegetarian":  {"शाकाहारी"},
		"vegan":       {"वीगन", "शुद्ध शाकाहारी"},
		"gluten-free": {"ग्लूटेन मुक्त", "ग्लूटेन फ्री"},
		"dairy-free":  {"डेयरी मुक्त", "बिना दूध"},
		"nut-free":    {"नट मुक्त", "बिना नट"},
	},
	"ja": {
		"vegetarian":  {"ベジタリアン", "菜食"},
		"vegan":       {"ヴィーガン", "ビーガン", "完全菜食"},
		"gluten-free": {"グルテンフリー"},
		"dairy-free":  {"乳製品不使用", "乳製品なし"},
		"nut-free":    {"ナッツ不使用", "ナッツなし"},
	},
	"ko": {
		"vegetarian":  {"채식"},
		"vegan":       {"비건", "완전 채식"},
		"gluten-free": {"글루텐 프리", "글루텐프리"},
		"dairy-free":  {"유제품 없", "유제품 무"},
		"nut-free":    {"견과류 없", "견과류 무"},
	},
	"th": {
		"vegetarian":  {"มังสวิรัติ"},
		"vegan":       {"วีแกน", "เจ"},
		"gluten-free": {"ปลอดกลูเตน", "ไม่มีกลูเตน"},
		"dairy-free":  {"ปลอดนม", "ไม่มีนม"},
		"nut-free":    {"ปลอดถั่ว", "ไม่มีถั่ว"},
	},
	"vi": {
		"vegetarian":  {"chay"},
		"vegan":       {"thuần chay"},
		"gluten-free": {"không gluten"},
		"dairy-free":  {"không sữa"},
		"nut-free":    {"không hạt"},
	},
	"zh": {
		"vegetarian":  {"素食", "素菜"},
		"vegan":       {"纯素", "純素", "全素"},
		"gluten-free": {"无麸质", "無麩質"},
		"dairy-free":  {"无乳制品", "無乳製品"},
		"nut-free":    {"无坚果", "無堅果"},
	},
}

// canonicalDietaryTagOrder keeps DetectDietaryTags deterministic regardless of
// Go map iteration order.
var canonicalDietaryTagOrder = []string{"vegan", "vegetarian", "gluten-free", "dairy-free", "nut-free"}

func waiterVocabulary(table map[string][]string, locale string) []string {
	fam := resolveWaiterLocale(locale).PromptFamily
	if values, ok := table[fam]; ok {
		return values
	}
	return table["en"]
}

func containsWaiterVocabulary(table map[string][]string, locale, text string) bool {
	lower := strings.ToLower(text)
	if lower == "" {
		return false
	}
	if waiterKeywordsMatch(waiterVocabulary(table, locale), lower) {
		return true
	}
	// Guests mix languages on a Spanish (or other) session. Scan English too
	// so "can I get the bill please?" still matches on an es/es-AR Sage.
	if resolveWaiterLocale(locale).PromptFamily != "en" {
		return waiterKeywordsMatch(table["en"], lower)
	}
	return false
}

func waiterKeywordsMatch(keywords []string, lower string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// DetectRecommendationIntent reports whether the guest is asking the waiter to
// pick something from the menu for them.
func DetectRecommendationIntent(locale, text string) bool {
	return containsWaiterVocabulary(recommendationKeywords, locale, text)
}

// DetectPairingIntent reports whether the guest is asking what goes with a dish
// they named. The caller must still resolve the anchor item from the menu
// snapshot before answering.
func DetectPairingIntent(locale, text string) bool {
	return containsWaiterVocabulary(pairingKeywords, locale, text)
}

// DetectAlternativeIntent reports whether the guest is asking for a substitute
// (typically because the dish they wanted is unavailable).
func DetectAlternativeIntent(locale, text string) bool {
	return containsWaiterVocabulary(alternativeKeywords, locale, text)
}

// DetectDietaryTags returns the canonical dietary tag IDs the guest asked for,
// in a deterministic order. "vegan" subsumes "vegetarian" so a vegan request in
// a locale whose vegetarian word is a substring of its vegan word (vi "chay" vs
// "thuần chay") does not over-constrain the filter.
func DetectDietaryTags(locale, text string) []string {
	lower := strings.ToLower(text)
	if lower == "" {
		return nil
	}
	fam := resolveWaiterLocale(locale).PromptFamily
	table, ok := dietaryKeywords[fam]
	if !ok {
		table = dietaryKeywords["en"]
	}
	matched := make(map[string]struct{}, len(canonicalDietaryTagOrder))
	for tag, phrases := range table {
		for _, phrase := range phrases {
			if phrase != "" && strings.Contains(lower, phrase) {
				matched[tag] = struct{}{}
				break
			}
		}
	}
	if _, vegan := matched["vegan"]; vegan {
		delete(matched, "vegetarian")
	}
	out := make([]string, 0, len(matched))
	for _, tag := range canonicalDietaryTagOrder {
		if _, ok := matched[tag]; ok {
			out = append(out, tag)
		}
	}
	return out
}

var soldOutKeywords = map[string][]string{
	"en":    {"86", "eighty six", "sold out", "what's unavailable", "whats unavailable", "what is unavailable", "out of stock", "inventory out", "esta 86", "está 86"},
	"es":    {"86", "agotado", "esta 86", "está 86", "sin stock", "no hay stock"},
	"es_ar": {"86", "agotado", "esta 86", "está 86", "sin stock", "no hay stock"},
}

var hoursKeywords = map[string][]string{
	// "when is it opn" covers the live guest typo behind issue 790c — full
	// phrases only, never the bare fragment, so item names stay unaffected.
	"en":    {"hours", "when do you close", "when do you open", "what time do you", "closing time", "opening time", "close tonight", "open tonight", "when is it open", "when is it opn", "when are you open", "are you open", "are u open", "open now", "still open", "open today", "opening hours", "what time are you open"},
	"es":    {"horario", "a que hora", "a qué hora", "cuando cierran", "cuándo cierran", "cuando abren", "cuándo abren", "hora de cierre", "hora de apertura", "cuando abre", "cuándo abre", "esta abierto", "está abierto", "abierto ahora", "abierto hoy"},
	"es_ar": {"horario", "a que hora", "a qué hora", "cuando cierran", "cuándo cierran", "cuando abren", "cuándo abren", "hora de cierre", "hora de apertura", "cuando abre", "cuándo abre", "esta abierto", "está abierto", "abierto ahora", "abierto hoy"},
}

// waitTimeKeywords route "how long it takes?" style kitchen-timing asks (issue
// 790b) to a follow-through answer instead of the capability shrug. Bare "how
// much" is deliberately absent — that phrasing asks for a price.
var waitTimeKeywords = map[string][]string{
	"en":    {"how long", "how much longer", "wait time", "waiting time", "how many minutes", "when will my", "when will it", "still waiting", "prep time"},
	"es":    {"cuanto tarda", "cuánto tarda", "cuanto demora", "cuánto demora", "cuanto falta", "cuánto falta", "cuanto tiempo", "cuánto tiempo", "cuando llega", "cuándo llega", "tiempo de espera"},
	"es_ar": {"cuanto tarda", "cuánto tarda", "cuanto demora", "cuánto demora", "cuanto falta", "cuánto falta", "cuanto tiempo", "cuánto tiempo", "cuando llega", "cuándo llega", "tiempo de espera"},
}

var reservationKeywords = map[string][]string{
	"en":    {"reservation", "reservations", "book a table", "booking", "reserve a table", "do you take reservations"},
	"es":    {"reserva", "reservas", "reservar", "tomar reserva"},
	"es_ar": {"reserva", "reservas", "reservar", "tomar reserva"},
}

var billKeywords = map[string][]string{
	"en":    {"the bill", "the check", "get the bill", "get the check", "can i pay", "check please", "bill please"},
	"es":    {"la cuenta", "la cuenta por favor", "traer la cuenta", "pedir la cuenta"},
	"es_ar": {"la cuenta", "la cuenta por favor", "traer la cuenta", "pedir la cuenta"},
}

var parkingKeywords = map[string][]string{
	"en":    {"parking", "park the car", "valet"},
	"es":    {"estacionamiento", "parking", "dónde estaciono", "donde estaciono"},
	"es_ar": {"estacionamiento", "parking", "dónde estaciono", "donde estaciono"},
}

var deliveryKeywords = map[string][]string{
	"en":    {"delivery", "deliver", "do you deliver"},
	"es":    {"delivery", "envío", "envio", "envian", "envían", "reparto"},
	"es_ar": {"delivery", "envío", "envio", "envian", "envían", "reparto"},
}

var serviceCallKeywords = map[string][]string{
	"en":    {"call the waiter", "call a waiter", "get the waiter", "call waiter", "need a waiter"},
	"es":    {"llama al mozo", "llamen al mozo", "llamar al mozo", "un mozo"},
	"es_ar": {"llama al mozo", "llamen al mozo", "llamar al mozo", "un mozo"},
}

var tablesKeywords = map[string][]string{
	"en":    {"tables free", "free tables", "tables available", "which tables", "open tables", "mesas libres", "mesas estan libres", "mesas están libres"},
	"es":    {"mesas libres", "mesas estan libres", "mesas están libres", "mesas disponibles", "que mesas", "qué mesas"},
	"es_ar": {"mesas libres", "mesas estan libres", "mesas están libres", "mesas disponibles", "que mesas", "qué mesas"},
}

// soldOutSkipWords are whole words that appear before an 86 / sold-out mention
// when the guest is excluding those dishes ("nothing 86'd please"), not asking
// the waiter to list them.
var soldOutSkipWords = map[string][]string{
	"en":    {"nothing", "no", "not", "without", "except", "none", "skip", "dont", "don't"},
	"es":    {"nada", "sin", "no", "ningun", "ningún"},
	"es_ar": {"nada", "sin", "no", "ningun", "ningún"},
}

var soldOutSkipPhrases = map[string][]string{
	"en":    {"please no", "please don't", "please dont"},
	"es":    {"que no este", "que no esté", "que no sea"},
	"es_ar": {"que no este", "que no esté", "que no sea"},
}

func soldOutVocabulary(locale string) []string {
	keywords := append([]string{}, waiterVocabulary(soldOutKeywords, locale)...)
	if resolveWaiterLocale(locale).PromptFamily != "en" {
		keywords = append(keywords, soldOutKeywords["en"]...)
	}
	return keywords
}

func eachSoldOutKeywordIndex(lower, keyword string, visit func(int) bool) bool {
	if keyword == "" || lower == "" {
		return false
	}
	start := 0
	for {
		rel := strings.Index(lower[start:], keyword)
		if rel < 0 {
			return false
		}
		pos := start + rel
		if visit(pos) {
			return true
		}
		start = pos + len(keyword)
	}
}

func collapseWaiterWordWindow(s string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		if unicode.IsLetter(r) || r == '\'' || r == '’' {
			b.WriteRune(unicode.ToLower(r))
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func soldOutMentionIsSkipConstraint(locale, lower string, pos int) bool {
	start := pos - 48
	if start < 0 {
		start = 0
	}
	window := collapseWaiterWordWindow(lower[start:pos])
	if window == "" {
		return false
	}
	phrases := append([]string{}, waiterVocabulary(soldOutSkipPhrases, locale)...)
	words := append([]string{}, waiterVocabulary(soldOutSkipWords, locale)...)
	if resolveWaiterLocale(locale).PromptFamily != "en" {
		phrases = append(phrases, soldOutSkipPhrases["en"]...)
		words = append(words, soldOutSkipWords["en"]...)
	}
	for _, phrase := range phrases {
		if phrase != "" && strings.Contains(window, phrase) {
			return true
		}
	}
	for _, field := range strings.Fields(window) {
		for _, word := range words {
			if word != "" && field == word {
				return true
			}
		}
	}
	return false
}

// DetectSoldOutIntent reports whether the guest is asking which dishes are 86'd
// / sold out. Mentions that only exclude those dishes ("nothing 86'd please")
// do not count — those are availability asks, not sold-out listings.
func DetectSoldOutIntent(locale, text string) bool {
	lower := strings.ToLower(text)
	if lower == "" {
		return false
	}
	for _, keyword := range soldOutVocabulary(locale) {
		foundListing := false
		eachSoldOutKeywordIndex(lower, keyword, func(pos int) bool {
			if !soldOutMentionIsSkipConstraint(locale, lower, pos) {
				foundListing = true
				return true
			}
			return false
		})
		if foundListing {
			return true
		}
	}
	return false
}

// DetectSoldOutSkipConstraint reports whether the guest asked to skip 86'd /
// sold-out dishes rather than list them.
func DetectSoldOutSkipConstraint(locale, text string) bool {
	lower := strings.ToLower(text)
	if lower == "" {
		return false
	}
	for _, keyword := range soldOutVocabulary(locale) {
		if eachSoldOutKeywordIndex(lower, keyword, func(pos int) bool {
			return soldOutMentionIsSkipConstraint(locale, lower, pos)
		}) {
			return true
		}
	}
	return false
}

func DetectHoursIntent(locale, text string) bool {
	return containsWaiterVocabulary(hoursKeywords, locale, text)
}

func DetectReservationIntent(locale, text string) bool {
	return containsWaiterVocabulary(reservationKeywords, locale, text)
}

func DetectBillIntent(locale, text string) bool {
	return containsWaiterVocabulary(billKeywords, locale, text)
}

func DetectParkingIntent(locale, text string) bool {
	return containsWaiterVocabulary(parkingKeywords, locale, text)
}

func DetectDeliveryIntent(locale, text string) bool {
	return containsWaiterVocabulary(deliveryKeywords, locale, text)
}

func DetectServiceCallIntent(locale, text string) bool {
	return containsWaiterVocabulary(serviceCallKeywords, locale, text)
}

func DetectTablesIntent(locale, text string) bool {
	return containsWaiterVocabulary(tablesKeywords, locale, text)
}

// DetectWaitTimeIntent reports whether the guest is asking how long their food
// (or the kitchen generally) will take.
func DetectWaitTimeIntent(locale, text string) bool {
	return containsWaiterVocabulary(waitTimeKeywords, locale, text)
}

// DietaryRecommendationIntent is a gluten/veg/etc. "what can I eat" ask, not a
// "does this named dish contain X?" allergen lookup.
func DietaryRecommendationIntent(locale, text string) bool {
	if len(DetectDietaryTags(locale, text)) == 0 {
		return false
	}
	// Broad discovery words ("anything gluten free?") are safe here because the
	// gate above already requires a detected dietary tag — issue 816.
	return DetectRecommendationIntent(locale, text) ||
		containsWaiterVocabulary(map[string][]string{
			"en":    {"what can i", "what do you have", "anything for", "options for", "anything", "something", "option", "is there", "do you have"},
			"es":    {"que hay", "qué hay", "que puedo", "qué puedo", "para celiac", "para celiaq", "algo", "tienen", "hay ", "alguna", "alguno", "opcion", "opción"},
			"es_ar": {"que hay", "qué hay", "que puedo", "qué puedo", "para celiac", "para celiaq", "algo", "tienen", "hay ", "alguna", "alguno", "opcion", "opción"},
		}, locale, text)
}

// DietaryTagAvoidedAllergens is the allergen vocabulary a dietary tag rules out.
// RecommendableEntities uses it as a fallback when no dish carries the tag:
// never recommend a dish that lists one of these, even if it is otherwise tagged.
func DietaryTagAvoidedAllergens(tag string) []string {
	switch strings.TrimSpace(strings.ToLower(tag)) {
	case "gluten-free":
		return []string{"gluten", "wheat", "trigo", "cebada", "centeno", "tacc"}
	case "dairy-free":
		return []string{"dairy", "milk", "lactose", "leche", "lacteo", "lácteo", "lacteos", "lácteos"}
	case "nut-free":
		return []string{"peanut", "peanuts", "tree nut", "tree nuts", "nut", "nuts", "mani", "maní", "cacahuate", "fruto seco", "nueces"}
	default:
		return nil
	}
}

// DietaryAskAvoidsAllergens reports whether any requested tag is an allergen-avoidance
// constraint (gluten-free / dairy-free / nut-free) rather than a cuisine tag.
func DietaryAskAvoidsAllergens(tags []string) bool {
	for _, tag := range tags {
		if len(DietaryTagAvoidedAllergens(tag)) > 0 {
			return true
		}
	}
	return false
}

// EntityContainsAvoidedAllergen reports whether the dish lists an allergen the
// guest asked to avoid. Empty allergen lists do not fail the check — tagged
// dishes still win; this only excludes positively unsafe rows.
func EntityContainsAvoidedAllergen(allergens, dietaryTags []string) bool {
	avoided := make([]string, 0, 8)
	for _, tag := range dietaryTags {
		avoided = append(avoided, DietaryTagAvoidedAllergens(tag)...)
	}
	if len(avoided) == 0 {
		return false
	}
	for _, allergen := range allergens {
		normalized := strings.ToLower(strings.TrimSpace(allergen))
		if normalized == "" {
			continue
		}
		for _, needle := range avoided {
			if needle != "" && strings.Contains(normalized, needle) {
				return true
			}
		}
	}
	return false
}

var excludedDishPhrases = map[string][]string{
	"en":    {"except ", "other than ", "that's not ", "that is not ", "not the "},
	"es":    {"no sea ", "que no sea ", "excepto ", "menos "},
	"es_ar": {"no sea ", "que no sea ", "excepto ", "menos "},
}

// ExtractExcludedDishName returns a guest-named dish they asked to skip
// ("que no sea milanesa"). Empty when the message has no exclusion phrase.
func ExtractExcludedDishName(locale, text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}
	fam := resolveWaiterLocale(locale).PromptFamily
	phrases, ok := excludedDishPhrases[fam]
	if !ok {
		phrases = excludedDishPhrases["en"]
	}
	for _, phrase := range phrases {
		idx := strings.Index(lower, phrase)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(lower[idx+len(phrase):])
		rest = strings.Trim(rest, " ?¿!.…,;")
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if len(fields) > 1 && len([]rune(fields[1])) <= 12 {
			name = fields[0] + " " + fields[1]
		}
		return strings.TrimSpace(name)
	}
	return ""
}

// WaiterJokePrompt reports whether the guest asked for a joke — the only
// waiter turn that still needs untrusted model prose.
func WaiterJokePrompt(locale, text string) bool {
	if locale != "en" && locale != "es" && locale != "es-AR" {
		return false
	}
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"tell me a joke", "make me laugh", "tell me something funny",
		"contame un chiste", "cuentame un chiste", "decime algo gracioso", "dime algo gracioso",
	} {
		if phrase != "" && strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
