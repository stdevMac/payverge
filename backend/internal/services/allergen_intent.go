package services

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

var allergenKeywords = map[string][]string{
	"en":    {"allerg", "gluten", "peanut", "tree nut", "nut", "dairy", "milk", "lactose", "egg", "fish", "shellfish", "crustacean", "shrimp", "soy", "sesame", "celery", "mustard", "sulphite", "sulfite", "lupin", "mollusc", "gluten free", "gluten-free", "nut free", "contain", "is it safe"},
	"es":    {"alerg", "gluten", "cacahuete", "maní", "fruto seco", "lácteo", "leche", "lactosa", "huevo", "pescado", "marisco", "crustáceo", "soja", "sésamo", "ajonjolí", "apio", "mostaza", "sulfito", "altramuz", "molusco", "sin gluten", "contiene", "es seguro"},
	"es_ar": {"alerg", "gluten", "maní", "fruto seco", "lácteo", "leche", "lactosa", "huevo", "pescado", "marisco", "crustáceo", "soja", "sésamo", "apio", "mostaza", "sulfito", "molusco", "sin gluten", "contiene", "lleva", "es seguro"},
	"fr":    {"allerg", "gluten", "arachide", "cacahuète", "fruits à coque", "laitier", "lait", "lactose", "œuf", "poisson", "crustacé", "soja", "sésame", "céleri", "moutarde", "sulfite", "lupin", "mollusque", "sans gluten", "contient", "est-ce sûr"},
	"de":    {"allerg", "gluten", "erdnuss", "nuss", "milch", "laktose", "ei", "eier", "fisch", "krustentier", "soja", "sesam", "sellerie", "senf", "sulfit", "lupine", "weichtier", "glutenfrei", "enthält", "ist es sicher"},
	"it":    {"allerg", "glutine", "arachide", "nocciola", "latte", "latte", "lattosio", "uovo", "pesce", "crostaceo", "soia", "sesamo", "sedano", "senape", "solfito", "lupino", "mollusco", "senza glutine", "contiene", "è sicuro"},
	"pt":    {"alerg", "glúten", "amendoim", "nozes", "lácteo", "leite", "lactose", "ovo", "peixe", "crustáceo", "soja", "sésamo", "gergelim", "aipo", "mostarda", "sulfito", "molusco", "sem glúten", "contém", "é seguro"},
	"nl":    {"allerg", "gluten", "pinda", "noot", "zuivel", "melk", "lactose", "ei", "eieren", "vis", "schaaldier", "soja", "sesam", "selderij", "mosterd", "sulfiet", "lupine", "weekdier", "glutenvrij", "bevat", "is het veilig"},
	"da":    {"allerg", "gluten", "jordnød", "nød", "nødde", "mejeri", "mælk", "laktose", "æg", "ægge", "fisk", "krebsdyr", "soja", "sesam", "selleri", "sennep", "sulfit", "lupin", "bløddyr", "glutenfri", "indeholder", "er det sikkert"},
	"no":    {"allerg", "gluten", "peanøtt", "nøtt", "meieri", "melk", "laktose", "egg", "fisk", "krepsdyr", "soya", "sesam", "selleri", "sennep", "sulfitt", "lupin", "bløtdyr", "glutenfri", "inneholder", "er det trygt"},
	"sv":    {"allerg", "gluten", "jordnöt", "nöt", "mejeri", "mjölk", "laktos", "ägg", "fisk", "kräftdjur", "soja", "sesam", "selleri", "senap", "sulfit", "lupin", "blötdjur", "glutenfri", "innehåller", "är det säkert"},
	"pl":    {"alerg", "gluten", "orzeszek", "orzech", "nabiał", "mleko", "laktoza", "jajko", "ryba", "skorupiak", "soja", "sezam", "seler", "musztarda", "siarczyn", "łubin", "mięczak", "bezglutenowa", "zawiera", "czy to bezpieczne"},
	"ru":    {"аллерг", "глютен", "арахис", "орех", "молочный", "молоко", "лактоза", "яйцо", "рыба", "ракообразный", "соя", "кунжут", "сельдерей", "горчица", "сульфит", "люпин", "моллюск", "без глютена", "содержит", "это безопасно"},
	"tr":    {"alerj", "gluten", "yer fıstığı", "fındık", "süt ürünü", "süt", "laktoz", "yumurta", "balık", "kabuklu", "soya", "susam", "kereviz", "hardal", "sülfit", "lüpen", "yumuşakça", "glütensiz", "içerir", "güvenli mi"},
	"ar":    {"حساسية", "غلوتين", "فول سوداني", "مكسرات", "ألبان", "حليب", "لاكتوز", "بيض", "سمك", "قشريات", "صويا", "سمسم", "كرفس", "خردل", "كبريتيت", "ترمس", "رخويات", "خال من الغلوتين", "يحتوي", "آمن"},
	"hi":    {"एलर्जी", "ग्लूटेन", "मूंगफली", "नट", "डेयरी", "दूध", "लैक्टोज", "अंडा", "मछली", "क्रस्टेशियन", "सोया", "तिल", "अजवाइन", "सरसों", "सल्फाइट", "ल्यूपिन", "मोलस्क", "ग्लूटेन मुक्त", "शामिल", "सुरक्षित"},
	"ja":    {"アレル", "グルテン", "ピーナッツ", "ナッツ", "乳製品", "牛乳", "乳糖", "卵", "魚", "甲殻類", "大豆", "ごま", "セロリ", "マスタード", "亜硫酸", "ルピナス", "軟体動物", "グルテンフリー", "含む", "安全"},
	"ko":    {"알레르", "글루텐", "땅콩", "견과류", "유제품", "우유", "유당", "계란", "생선", "갑각류", "대두", "참깨", "셀러리", "겨자", "아황산", "루핀", "연체동물", "글루텐 프리", "함유", "안전한가"},
	"th":    {"แพ้", "กลูเตน", "ถั่วลิสง", "ถั่ว", "นม", "แลคโตส", "ไข่", "ปลา", "สัตว์น้ำ", "ถั่วเหลือง", "งา", "เซเลอรี", "มัสตาร์ด", "ซัลไฟต์", "ลูพิน", "หอย", "ปลอดกลูเตน", "ส่วนผสม", "ปลอดภัย"},
	"vi":    {"dị ứng", "gluten", "đậu phộng", "lạc", "hạt", "sữa", "sữa bò", "lactose", "trứng", "cá", "giáp xác", "đậu nành", "mè", "cần tây", "mù tạt", "sulfit", "lupin", "nhuyễn thể", "không gluten", "chứa", "an toàn"},
	"zh":    {"过敏", "麸质", "花生", "坚果", "乳制品", "牛奶", "乳糖", "鸡蛋", "鱼", "甲壳", "大豆", "芝麻", "芹菜", "芥末", "亚硫酸", "羽扇豆", "软体动物", "无麸质", "含有", "安全"},
}

// allergenWholeTokenKeywords lists, per prompt family, keywords so short they
// hide inside everyday words of that language (de/nl "ei" in "eine"/"eigenlijk",
// da "nød" in "nødvendig", fr "lait" in "laitue", vi "cá" in "các"). These must
// match a complete token — letter boundary on BOTH sides — instead of just a
// token prefix. Inflected/compound forms are covered by explicit stem keywords
// ("eier", "eieren", "ægge", "nødde") in allergenKeywords.
var allergenWholeTokenKeywords = map[string]map[string]bool{
	"de": {"ei": true},
	"nl": {"ei": true},
	"da": {"æg": true, "nød": true},
	"fr": {"lait": true},
	"vi": {"cá": true, "mè": true},
}

// allergenMatcher is one prepared keyword: lowercased and Latin-diacritic
// folded once at package init so the hot path only folds the guest message.
type allergenMatcher struct {
	keyword    string
	wholeToken bool
}

// allergenMatchers mirrors allergenKeywords with every keyword lowercased and
// diacritic-folded, carrying its allergenWholeTokenKeywords flag along.
var allergenMatchers = buildAllergenMatchers()

func buildAllergenMatchers() map[string][]allergenMatcher {
	out := make(map[string][]allergenMatcher, len(allergenKeywords))
	for fam, kws := range allergenKeywords {
		whole := allergenWholeTokenKeywords[fam]
		list := make([]allergenMatcher, 0, len(kws))
		for _, kw := range kws {
			if kw == "" {
				continue
			}
			folded := foldLatinDiacritics(strings.ToLower(kw))
			if folded == "" {
				continue
			}
			list = append(list, allergenMatcher{keyword: folded, wholeToken: whole[kw]})
		}
		out[fam] = list
	}
	return out
}

// foldLatinDiacritics strips combining marks that sit on a LATIN base letter,
// so "alérgico" → "alergico" and "glúten" → "gluten". Applied to the guest
// message AND to the keyword list, it makes matching accent-insensitive in both
// directions: accented guest phrasing reaches unaccented stems ("alerg") and
// unaccented typing reaches accented keywords ("glúten", "lácteo") (#950).
//
// The Latin-base condition is load-bearing. Thai tone marks, Devanagari matras
// and Arabic harakat are also category Mn, but they are letters there — a
// blanket runes.Remove(runes.In(unicode.Mn)) fuses unrelated syllables and
// invents keywords the guest never wrote (Thai "มีเมนู..." would sprout
// "นม"/milk). Non-Latin scripts therefore round-trip unchanged.
func foldLatinDiacritics(s string) string {
	if isASCIIOnly(s) {
		return s
	}
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(decomposed))
	latinBase := false
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			if latinBase {
				continue
			}
			b.WriteRune(r)
			continue
		}
		latinBase = unicode.Is(unicode.Latin, r)
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

func isASCIIOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// DetectAllergenIntent reports whether the guest message is an allergen-safety
// question, using the locale-family vocabulary (falls back to en).
//
// DELIBERATE OVER-TRIGGER, WITH TOKEN BOUNDARIES: keywords still match broadly
// (stem/prefix, e.g. "allerg" → "allergy", "peanut" → "peanuts", "nuss" →
// "nussallergie") and WILL false-positive on real allergen words — that is
// intentional and safe, since a false positive only routes the query to the
// deterministic allergen path. But a keyword may no longer match in the MIDDLE
// of another word: "egg" inside "veggie" must not hijack a dietary question
// into the allergen refusal (#596). Matching is Unicode letter-boundary aware;
// scripts written without spaces (Han, kana, Thai) and Arabic (attached
// clitics: "والحليب") keep plain substring semantics.
//
// ACCENT-INSENSITIVE (#950): both sides are Latin-diacritic folded first, so
// natural accented Spanish/Portuguese ("soy alérgico", "¿tiene alérgenos?",
// "sou alérgico") reaches the "alerg" stem instead of falling through to the
// generic food path with no staff-confirmation disclaimer attached.
func DetectAllergenIntent(locale, text string) bool {
	fam := resolveWaiterLocale(locale).PromptFamily
	matchers, ok := allergenMatchers[fam]
	if !ok {
		matchers = allergenMatchers["en"]
	}
	folded := foldLatinDiacritics(strings.ToLower(text))
	for _, m := range matchers {
		if containsKeywordAtLetterBoundary(folded, m.keyword, m.wholeToken) {
			return true
		}
	}
	return false
}

// letterBoundaryExempt reports whether a rune belongs to a script written
// without spaces between words (Han, Hiragana, Katakana, Thai) or one whose
// clitics attach directly to the word (Arabic: "والحليب" = "and the milk").
// For these scripts an ASCII-style word boundary would reject virtually every
// legitimate match, so matching stays plain-substring there.
func letterBoundaryExempt(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Thai, r) ||
		unicode.Is(unicode.Arabic, r)
}

// isTokenBoundaryRune: anything that is not a letter separates tokens; runes
// of boundary-exempt scripts always count as a boundary.
func isTokenBoundaryRune(r rune) bool {
	return !unicode.IsLetter(r) || letterBoundaryExempt(r)
}

// containsKeywordAtLetterBoundary reports whether kw occurs in lower starting
// at a token boundary (the rune before the match is not a letter). When
// wholeToken is set the match must also END at a token boundary, so very short
// keywords ("ei", "cá") cannot match as a prefix of an unrelated word. If the
// keyword's edge runes are from a boundary-exempt script the corresponding
// check is skipped (plain substring semantics).
func containsKeywordAtLetterBoundary(lower, kw string, wholeToken bool) bool {
	first, _ := utf8.DecodeRuneInString(kw)
	last, _ := utf8.DecodeLastRuneInString(kw)
	for offset := 0; offset < len(lower); {
		idx := strings.Index(lower[offset:], kw)
		if idx < 0 {
			return false
		}
		start := offset + idx
		end := start + len(kw)

		leftOK := start == 0 || letterBoundaryExempt(first)
		if !leftOK {
			prev, _ := utf8.DecodeLastRuneInString(lower[:start])
			leftOK = isTokenBoundaryRune(prev)
		}
		rightOK := true
		if wholeToken && end < len(lower) && !letterBoundaryExempt(last) {
			next, _ := utf8.DecodeRuneInString(lower[end:])
			rightOK = isTokenBoundaryRune(next)
		}
		if leftOK && rightOK {
			return true
		}
		// Advance by one rune so overlapping occurrences are still examined.
		_, size := utf8.DecodeRuneInString(lower[start:])
		if size == 0 {
			size = 1
		}
		offset = start + size
	}
	return false
}

var allergenDisclaimers = map[string]string{
	"en":    "Please confirm with our staff before ordering — our allergen information may be incomplete.",
	"es":    "Por favor, confirma con nuestro personal antes de pedir — nuestra información de alérgenos puede estar incompleta.",
	"es-AR": "Por favor, confirmá con nuestro personal antes de pedir — la información de alérgenos puede estar incompleta.",
	"fr":    "Veuillez confirmer avec notre personnel avant de commander — nos informations sur les allergènes peuvent être incomplètes.",
	"de":    "Bitte bestätigen Sie mit unserem Personal vor der Bestellung — unsere Allergeninformationen können unvollständig sein.",
	"it":    "Si prega di confermare con il nostro personale prima di ordinare — le nostre informazioni sugli allergeni potrebbero essere incomplete.",
	"pt":    "Por favor, confirme com nossa equipe antes de pedir — nossas informações sobre alérgenos podem estar incompletas.",
	"nl":    "Bevestig alstublieft met ons personeel voordat u bestelt — onze allergeeninformatie kan onvolledig zijn.",
	"da":    "Bekræft venligst med vores personale før bestilling — vores allergenoplysninger kan være ufuldstændige.",
	"no":    "Vennligst bekreft med vårt personale før bestilling — vår allergeninformasjon kan være ufullstendig.",
	"sv":    "Vänligen bekräfta med vår personal innan beställning — vår allergeninformation kan vara ofullständig.",
	"pl":    "Proszę potwierdzić z naszym personelem przed zamówieniem — nasze informacje o alergenach mogą być niekompletne.",
	"ru":    "Пожалуйста, подтвердите с нашим персоналом перед заказом — наша информация об аллергенах может быть неполной.",
	"tr":    "Lütfen sipariş vermeden önce personelimizle teyit edin — alerjen bilgilerimiz eksik olabilir.",
	"ar":    "يرجى التأكيد مع موظفينا قبل الطلب — قد تكون معلومات المواد المسببة للحساسية لدينا غير كاملة.",
	"hi":    "कृपया ऑर्डर करने से पहले हमारे स्टाफ से पुष्टि करें — हमारी एलर्जी की जानकारी अधूरी हो सकती है।",
	"ja":    "ご注文前にスタッフにご確認ください — アレルゲン情報が不完全な場合があります。",
	"ko":    "주문 전에 직원에게 확인하십시오 — 알레르기 유발 물질 정보가 불완전할 수 있습니다.",
	"th":    "โปรดยืนยันกับพนักงานของเราก่อนสั่ง — ข้อมูลสารก่อภูมิแพ้อาจไม่สมบูรณ์",
	"vi":    "Vui lòng xác nhận với nhân viên của chúng tôi trước khi đặt hàng — thông tin về chất gây dị ứng có thể không đầy đủ.",
	"zh":    "请在点餐前向我们的工作人员确认 — 过敏原信息可能不完整。",
}

var allergenRefusals = map[string]string{
	"en":    "I can't confirm the allergen details for that item, so please check with our staff before ordering to stay safe.",
	"es":    "No puedo confirmar los alérgenos de ese plato, así que por favor consulta con nuestro personal antes de pedir.",
	"es-AR": "No puedo confirmar los alérgenos de ese plato, así que por favor consultá con nuestro personal antes de pedir.",
	"fr":    "Je ne peux pas confirmer les détails des allergènes pour cet article, veuillez vérifier avec notre personnel avant de commander.",
	"de":    "Ich kann die Allergendetails für diesen Artikel nicht bestätigen, bitte fragen Sie vor der Bestellung unser Personal.",
	"it":    "Non posso confermare i dettagli sugli allergeni per questo articolo, quindi controlla con il nostro personale prima di ordinare.",
	"pt":    "Não posso confirmar os detalhes de alérgenos para este item, então verifique com nossa equipe antes de pedir.",
	"nl":    "Ik kan de allergeendetails voor dat item niet bevestigen, dus controleer alstublieft met ons personeel voordat u bestelt.",
	"da":    "Jeg kan ikke bekræfte allergenoplysningerne for den vare, så tjek venligst med vores personale før bestilling.",
	"no":    "Jeg kan ikke bekrefte allergenopplysningene for den varen, så sjekk med vårt personale før du bestiller.",
	"sv":    "Jag kan inte bekräfta allergenuppgifterna för den varan, så kolla med vår personal innan du beställer.",
	"pl":    "Nie mogę potwierdzić szczegółów dotyczących alergenów dla tego produktu, więc przed zamówieniem skonsultuj się z naszym personelem.",
	"ru":    "Я не могу подтвердить информацию об аллергенах для этого блюда, пожалуйста, уточните у нашего персонала перед заказом.",
	"tr":    "Bu ürün için alerjen detaylarını onaylayamıyorum, bu yüzden sipariş vermeden önce personelimize danışın.",
	"ar":    "لا يمكنني تأكيد تفاصيل المواد المسببة للحساسية لهذا العنصر، لذا يرجى التحقق مع موظفينا قبل الطلب.",
	"hi":    "मैं उस आइटम के लिए एलर्जी विवरण की पुष्टि नहीं कर सकता, इसलिए कृपया ऑर्डर करने से पहले हमारे स्टाफ से जांच करें।",
	"ja":    "その商品のアレルゲン詳細を確認できませんので、ご注文前にスタッフにご確認ください。",
	"ko":    "해당 품목의 알레르기 유발 물질 세부 정보를 확인할 수 없으므로 주문 전에 직원에게 확인하십시오.",
	"th":    "ฉันไม่สามารถยืนยันรายละเอียดสารก่อภูมิแพ้สำหรับรายการนั้นได้ โปรดตรวจสอบกับพนักงานของเราก่อนสั่ง",
	"vi":    "Tôi không thể xác nhận chi tiết chất gây dị ứng cho món đó, vui lòng kiểm tra với nhân viên của chúng tôi trước khi đặt hàng.",
	"zh":    "我无法确认该菜品的过敏原详情，请在点餐前向我们的工作人员确认。",
}

func allergenCopy(m map[string]string, code string) string {
	if v, ok := m[code]; ok {
		return v
	}
	if loc, ok2 := locales.Lookup(code); ok2 {
		if v, ok3 := m[loc.Canonical]; ok3 {
			return v
		}
	}
	return m["en"]
}

// AllergenDisclaimer returns the localized "confirm with staff" line.
func AllergenDisclaimer(code string) string { return allergenCopy(allergenDisclaimers, code) }

// AllergenRefusal returns the localized refusal used when data is missing.
func AllergenRefusal(code string) string { return allergenCopy(allergenRefusals, code) }

// clarifyItemMessages is the localized "I couldn't find that on the menu"
// clarification the AI waiter emits when it drops an invalid add_to_cart call.
var clarifyItemMessages = map[string]string{
	"en":    "I couldn't find that on the menu — could you tell me which item you'd like?",
	"es":    "No encontré eso en el menú — ¿podrías decirme qué artículo quieres?",
	"es-AR": "No encontré eso en el menú — ¿me podés decir qué producto querés?",
	"fr":    "Je n'ai pas trouvé cela sur le menu — pourriez-vous me dire quel article vous souhaitez ?",
	"de":    "Ich konnte das nicht auf der Speisekarte finden — welchen Artikel möchten Sie?",
	"it":    "Non l'ho trovato nel menu — potresti dirmi quale articolo desideri?",
	"pt":    "Não encontrei isso no cardápio — pode me dizer qual item você quer?",
	"nl":    "Ik kon dat niet op het menu vinden — welk item wil je graag?",
	"da":    "Jeg kunne ikke finde det på menuen — hvilken vare vil du gerne have?",
	"no":    "Jeg fant ikke det på menyen — hvilken vare vil du gjerne ha?",
	"sv":    "Jag kunde inte hitta det på menyn — vilken vara vill du ha?",
	"pl":    "Nie znalazłem tego w menu — który produkt chciałbyś zamówić?",
	"ru":    "Я не нашёл это в меню — подскажите, какое блюдо вы хотите?",
	"tr":    "Bunu menüde bulamadım — hangi ürünü istersiniz?",
	"ar":    "لم أجد ذلك في القائمة — هل يمكنك إخباري بالعنصر الذي تريده؟",
	"hi":    "मुझे वह मेन्यू में नहीं मिला — क्या आप बता सकते हैं कि आप कौन सा आइटम चाहते हैं?",
	"ja":    "メニューに見つかりませんでした — どの商品をご希望か教えていただけますか？",
	"ko":    "메뉴에서 찾을 수 없었습니다 — 어떤 항목을 원하시는지 알려주시겠어요?",
	"th":    "ฉันไม่พบรายการนั้นในเมนู — ช่วยบอกได้ไหมว่าคุณต้องการรายการใด",
	"vi":    "Tôi không tìm thấy món đó trong thực đơn — bạn muốn gọi món nào?",
	"zh":    "我在菜单上找不到那个 — 您想要哪个商品呢？",
}

// ClarifyItemMessage returns the localized menu-clarification prompt.
func ClarifyItemMessage(code string) string { return allergenCopy(clarifyItemMessages, code) }

// orderingPausedMessages: guest-facing reply when the model tries to add to cart
// while the venue is closed or guest ordering is off (Closed Mode).
var orderingPausedMessages = map[string]string{
	"en":    "Ordering is paused right now — you can still ask me anything about the menu.",
	"es":    "Los pedidos están en pausa ahora — aún puedes preguntarme lo que quieras sobre el menú.",
	"es-AR": "Los pedidos están en pausa ahora — todavía podés preguntarme lo que quieras sobre el menú.",
	"fr":    "Les commandes sont en pause pour le moment — vous pouvez toujours me poser des questions sur le menu.",
	"de":    "Bestellungen sind gerade pausiert — Sie können mich weiterhin zum Menü fragen.",
	"it":    "Gli ordini sono in pausa al momento — puoi comunque chiedermi del menu.",
	"pt":    "Os pedidos estão pausados agora — você ainda pode me perguntar qualquer coisa sobre o cardápio.",
	"nl":    "Bestellen is nu gepauzeerd — u kunt me nog steeds vragen stellen over het menu.",
	"da":    "Bestilling er sat på pause lige nu — du kan stadig spørge mig om menuen.",
	"no":    "Bestilling er satt på pause nå — du kan fortsatt spørre meg om menyen.",
	"sv":    "Beställning är pausad just nu — du kan fortfarande fråga mig om menyn.",
	"pl":    "Zamawianie jest teraz wstrzymane — nadal możesz pytać mnie o menu.",
	"ru":    "Заказы сейчас на паузе — вы всё ещё можете спрашивать меня о меню.",
	"tr":    "Sipariş şu an duraklatıldı — menü hakkında hâlâ soru sorabilirsiniz.",
	"ar":    "الطلب متوقف حالياً — ما زال بإمكانك سؤالي عن القائمة.",
	"hi":    "अभी ऑर्डर रोक दिया गया है — आप मुझसे मेनू के बारे में कुछ भी पूछ सकते हैं।",
	"ja":    "現在注文は受け付けていませんが、メニューについて何でもお聞きください。",
	"ko":    "지금은 주문이 일시 중지되어 있습니다 — 메뉴에 대해 무엇이든 물어보세요.",
	"th":    "ขณะนี้หยุดรับออเดอร์ชั่วคราว — ถามฉันเกี่ยวกับเมนูได้เลย",
	"vi":    "Hiện tạm dừng đặt món — bạn vẫn có thể hỏi tôi về thực đơn.",
	"zh":    "目前暂停点餐——欢迎询问菜单相关问题。",
}

// OrderingPausedMessage returns the localized closed-hours / ordering-off reply.
func OrderingPausedMessage(code string) string {
	return allergenCopy(orderingPausedMessages, code)
}

// AppendAllergenDisclaimer appends the localized disclaimer once (idempotent).
func AppendAllergenDisclaimer(code, text string) string {
	d := AllergenDisclaimer(code)
	if strings.Contains(text, d) {
		return text
	}
	t := strings.TrimRight(text, " \n")
	if t == "" {
		return d
	}
	return t + "\n\n" + d
}

// staffConfirmationMarkers: per-family {staffWords, confirmWords}. Derived from the
// existing allergenDisclaimers/allergenRefusals copy. A match requires at least one
// staff word AND one confirm word, so a lone mention never suppresses the disclaimer.
var staffConfirmationStaffWords = map[string][]string{
	"en": {"staff"}, "es": {"personal"}, "es_ar": {"personal"}, "fr": {"personnel"},
	"de": {"personal"}, "it": {"personale"}, "pt": {"equipe", "pessoal"}, "nl": {"personeel"},
	"da": {"personale"}, "no": {"personale"}, "sv": {"personal"}, "pl": {"personel"},
	"ru": {"персонал"}, "tr": {"personel"}, "ar": {"موظف"}, "hi": {"स्टाफ"},
	"ja": {"スタッフ"}, "ko": {"직원"}, "th": {"พนักงาน"}, "vi": {"nhân viên"}, "zh": {"工作人员", "员工"},
}

var staffConfirmationConfirmWords = map[string][]string{
	"en": {"confirm", "check", "ask"}, "es": {"confirm", "consult"}, "es_ar": {"confirm", "consult"},
	"fr": {"confirm", "vérif"}, "de": {"bestätig", "frag"}, "it": {"conferma", "controll", "verific"},
	"pt": {"confirm", "verifi"}, "nl": {"bevestig", "controleer"}, "da": {"bekræft", "tjek"},
	"no": {"bekreft", "sjekk"}, "sv": {"bekräfta", "kolla"}, "pl": {"potwierdz", "skonsultuj", "sprawdź"},
	"ru": {"подтверд", "уточн", "проверь"}, "tr": {"teyit", "danış", "kontrol"},
	"ar": {"تأكيد", "تحقق", "اسأل"}, "hi": {"पुष्टि", "जांच"}, "ja": {"確認"}, "ko": {"확인"},
	"th": {"ยืนยัน", "ตรวจสอบ"}, "vi": {"xác nhận", "kiểm tra"}, "zh": {"确认", "咨询"},
}

func anyContains(haystack string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// MentionsStaffConfirmation reports whether text already tells the guest to confirm
// allergens with staff (in the given locale), so the post-check does not append a
// duplicate disclaimer. Conservative: a lone staff or lone confirm word is NOT enough.
func MentionsStaffConfirmation(code, text string) bool {
	if text == "" {
		return false
	}
	if strings.Contains(text, AllergenDisclaimer(code)) || strings.Contains(text, AllergenRefusal(code)) {
		return true
	}
	fam := resolveWaiterLocale(code).PromptFamily
	staff, ok := staffConfirmationStaffWords[fam]
	if !ok {
		staff = staffConfirmationStaffWords["en"]
	}
	confirm, ok := staffConfirmationConfirmWords[fam]
	if !ok {
		confirm = staffConfirmationConfirmWords["en"]
	}
	lower := strings.ToLower(text)
	return anyContains(lower, lowerAll(staff)) && anyContains(lower, lowerAll(confirm))
}

// AllergenAnswerFromItem builds a deterministic, localized allergen answer from
// a menu item's explicit allergens, with the disclaimer appended. Empty
// allergens -> refusal copy.
func AllergenAnswerFromItem(code, itemName string, allergens []string) string {
	if len(allergens) == 0 {
		return AllergenRefusal(code)
	}
	body := itemName + ": " + strings.Join(allergens, ", ") + "."
	return AppendAllergenDisclaimer(code, body)
}
