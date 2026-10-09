package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

const waiterVisitPlaceholder = "{item}"

// WaiterVisitFacts is trusted, server-computed venue state for hours, bills,
// and reservations. The waiter finalizer may quote these strings; they must
// never include model prose.
type WaiterVisitFacts struct {
	HoursKnown   bool
	TodayClosed  bool
	TodayOpen    string
	TodayClose   string
	Reservations bool
	PartyMin     int
	PartyMax     int
	HasOpenBill  bool
	BillSummary  string

	// Delivery* is the venue's real delivery configuration, the same evidence
	// the Delivery settings screen shows. DeliveryKnown separates "this venue
	// does not deliver" from "we could not read the settings" — only the first
	// is safe to state as fact. This field set replaces DeliveryHint, which
	// carried the model's English system-prompt fragment straight to guests
	// (#903), and closes the same grounding gap #871 closed for the Director.
	DeliveryKnown    bool
	DeliveryEnabled  bool
	DeliveryInHouse  bool
	DeliveryPartners []string
	// DeliveryFeeLabel is pre-formatted in the venue currency by the caller,
	// the same way BillSummary is, so this package never re-derives money.
	DeliveryFeeLabel string
}

func waiterVisitCopy(m map[string]string, code string) string {
	if v, ok := m[code]; ok {
		return v
	}
	if loc, ok := locales.Lookup(code); ok {
		if v, ok := m[loc.Canonical]; ok {
			return v
		}
	}
	return m["en"]
}

func waiterVisitWithItem(template, name string) string {
	if !strings.Contains(template, waiterVisitPlaceholder) {
		return template
	}
	return strings.ReplaceAll(template, waiterVisitPlaceholder, name)
}

var hoursOpenCopy = map[string]string{
	"en":    "We're open {open}–{close} today.",
	"es":    "Hoy abrimos de {open} a {close}.",
	"es-AR": "Hoy abrimos de {open} a {close}.",
	"fr":    "Nous sommes ouverts de {open} à {close} aujourd'hui.",
	"de":    "Wir haben heute von {open} bis {close} geöffnet.",
	"it":    "Oggi siamo aperti dalle {open} alle {close}.",
	"pt":    "Hoje estamos abertos das {open} às {close}.",
	"nl":    "We zijn vandaag open van {open} tot {close}.",
	"da":    "Vi har åbent i dag fra {open} til {close}.",
	"no":    "Vi har åpent i dag fra {open} til {close}.",
	"sv":    "Vi har öppet idag {open}–{close}.",
	"pl":    "Dziś jesteśmy otwarci {open}–{close}.",
	"ru":    "Сегодня мы работаем с {open} до {close}.",
	"tr":    "Bugün {open}–{close} arasındayız.",
	"ar":    "نحن مفتوحون اليوم من {open} إلى {close}.",
	"hi":    "आज हम {open}–{close} खुले हैं।",
	"ja":    "本日の営業は {open}〜{close} です。",
	"ko":    "오늘은 {open}–{close}에 영업합니다.",
	"th":    "วันนี้เปิด {open}–{close}",
	"vi":    "Hôm nay chúng tôi mở cửa {open}–{close}.",
	"zh":    "今天营业时间 {open}–{close}。",
}

// hoursOpenAllDayCopy answers a day whose single window opens and closes at
// the same time (the demo seeds 00:00-00:00). Every hours reader treats
// close <= open as wrapping past midnight, so an equal pair is a full 24 hours;
// quoting it as "open 00:00-00:00" reads to a guest like a closed venue.
var hoursOpenAllDayCopy = map[string]string{
	"en":    "We're open 24 hours today.",
	"es":    "Hoy abrimos las 24 horas.",
	"es-AR": "Hoy abrimos las 24 horas.",
	"fr":    "Nous sommes ouverts 24 heures sur 24 aujourd'hui.",
	"de":    "Wir haben heute rund um die Uhr geöffnet.",
	"it":    "Oggi siamo aperti 24 ore su 24.",
	"pt":    "Hoje estamos abertos 24 horas.",
	"nl":    "We zijn vandaag 24 uur per dag open.",
	"da":    "Vi har åbent hele døgnet i dag.",
	"no":    "Vi har åpent hele døgnet i dag.",
	"sv":    "Vi har öppet dygnet runt idag.",
	"pl":    "Dziś jesteśmy otwarci całą dobę.",
	"ru":    "Сегодня мы работаем круглосуточно.",
	"tr":    "Bugün 24 saat açığız.",
	"ar":    "نحن مفتوحون اليوم على مدار 24 ساعة.",
	"hi":    "आज हम 24 घंटे खुले हैं।",
	"ja":    "本日は24時間営業です。",
	"ko":    "오늘은 24시간 영업합니다.",
	"th":    "วันนี้เปิด 24 ชั่วโมง",
	"vi":    "Hôm nay chúng tôi mở cửa 24 giờ.",
	"zh":    "今天24小时营业。",
}

var hoursClosedCopy = map[string]string{
	"en":    "We're closed today.",
	"es":    "Hoy estamos cerrados.",
	"es-AR": "Hoy estamos cerrados.",
	"fr":    "Nous sommes fermés aujourd'hui.",
	"de":    "Wir haben heute geschlossen.",
	"it":    "Oggi siamo chiusi.",
	"pt":    "Hoje estamos fechados.",
	"nl":    "We zijn vandaag gesloten.",
	"da":    "Vi har lukket i dag.",
	"no":    "Vi har stengt i dag.",
	"sv":    "Vi har stängt idag.",
	"pl":    "Dziś jesteśmy zamknięci.",
	"ru":    "Сегодня мы закрыты.",
	"tr":    "Bugün kapalıyız.",
	"ar":    "نحن مغلقون اليوم.",
	"hi":    "आज हम बंद हैं।",
	"ja":    "本日は定休日です。",
	"ko":    "오늘은 쉽니다.",
	"th":    "วันนี้ปิด",
	"vi":    "Hôm nay chúng tôi đóng cửa.",
	"zh":    "今天休息。",
}

var hoursUnknownCopy = map[string]string{
	"en":    "I don't have today's hours here — please ask our staff.",
	"es":    "No tengo el horario de hoy aquí — por favor consulta con nuestro personal.",
	"es-AR": "No tengo el horario de hoy acá — consultá con nuestro personal.",
	"fr":    "Je n'ai pas les horaires d'aujourd'hui — demandez à notre personnel.",
	"de":    "Ich habe die heutigen Öffnungszeiten hier nicht — bitte fragen Sie unser Personal.",
	"it":    "Non ho gli orari di oggi — chiedi al nostro personale.",
	"pt":    "Não tenho o horário de hoje aqui — pergunte à nossa equipe.",
	"nl":    "Ik heb de openingstijden van vandaag hier niet — vraag het ons personeel.",
	"da":    "Jeg har ikke dagens åbningstider her — spørg venligst personalet.",
	"no":    "Jeg har ikke dagens åpningstider her — spør personalet.",
	"sv":    "Jag har inte dagens öppettider här — fråga personalen.",
	"pl":    "Nie mam tu dzisiejszych godzin — zapytaj personel.",
	"ru":    "У меня нет сегодняшнего расписания — уточните у персонала.",
	"tr":    "Bugünün saatleri burada yok — lütfen personelimize sorun.",
	"ar":    "ليست لدي ساعات اليوم هنا — يرجى سؤال موظفينا.",
	"hi":    "आज के समय यहाँ नहीं हैं — कृपया स्टाफ से पूछें।",
	"ja":    "本日の営業時間がわかりません — スタッフにお尋ねください。",
	"ko":    "오늘 영업시간이 여기 없습니다 — 직원에게 물어보세요.",
	"th":    "ฉันไม่มีเวลาเปิดวันนี้ที่นี่ — ถามพนักงานได้เลย",
	"vi":    "Tôi không có giờ hôm nay tại đây — hãy hỏi nhân viên.",
	"zh":    "这里没有今天的营业时间——请向工作人员询问。",
}

var reservationEnabledCopy = map[string]string{
	"en":    "We take reservations for parties of {min} to {max}. Use Reserve a table on this page.",
	"es":    "Tomamos reservas para grupos de {min} a {max} personas. Usá Reservar mesa en esta página.",
	"es-AR": "Tomamos reservas para grupos de {min} a {max} personas. Usá Reservar mesa en esta página.",
	"fr":    "Nous prenons des réservations pour {min} à {max} personnes. Utilisez Réserver une table sur cette page.",
	"de":    "Wir nehmen Reservierungen für {min} bis {max} Personen entgegen. Nutzen Sie Tisch reservieren auf dieser Seite.",
	"it":    "Accettiamo prenotazioni per gruppi da {min} a {max} persone. Usa Prenota un tavolo in questa pagina.",
	"pt":    "Aceitamos reservas para grupos de {min} a {max} pessoas. Use Reservar mesa nesta página.",
	"nl":    "We nemen reserveringen aan voor {min} tot {max} personen. Gebruik Reserveer een tafel op deze pagina.",
	"da":    "Vi tager reservationer til {min}–{max} personer. Brug Reservér et bord på denne side.",
	"no":    "Vi tar reservasjoner for {min}–{max} personer. Bruk Reserver et bord på denne siden.",
	"sv":    "Vi tar reservationer för {min}–{max} personer. Använd Boka bord på den här sidan.",
	"pl":    "Przyjmujemy rezerwacje dla {min}–{max} osób. Użyj Zarezerwuj stolik na tej stronie.",
	"ru":    "Мы принимаем брони для компаний от {min} до {max} человек. Нажмите Забронировать стол на этой странице.",
	"tr":    "{min}–{max} kişilik rezervasyon alıyoruz. Bu sayfadaki Masa ayırtın seçeneğini kullanın.",
	"ar":    "نقبل الحجوزات لمجموعات من {min} إلى {max}. استخدم احجز طاولة في هذه الصفحة.",
	"hi":    "हम {min} से {max} लोगों की बुकिंग लेते हैं। इस पेज पर टेबल रिज़र्व करें।",
	"ja":    "{min}〜{max}名のご予約を受け付けています。このページの「テーブルを予約」をご利用ください。",
	"ko":    "{min}–{max}명 예약을 받습니다. 이 페이지의 테이블 예약을 이용하세요.",
	"th":    "รับจองสำหรับ {min}–{max} คน ใช้ จองโต๊ะ ในหน้านี้",
	"vi":    "Chúng tôi nhận đặt chỗ cho {min}–{max} người. Dùng Đặt bàn trên trang này.",
	"zh":    "我们接受 {min} 至 {max} 人的预订。请使用本页的预订餐桌。",
}

var reservationDisabledCopy = map[string]string{
	"en":    "We don't take reservations through this chat — please ask our staff.",
	"es":    "No tomamos reservas por este chat — por favor consulta con nuestro personal.",
	"es-AR": "No tomamos reservas por este chat — consultá con nuestro personal.",
	"fr":    "Nous ne prenons pas de réservation via ce chat — demandez à notre personnel.",
	"de":    "Über diesen Chat nehmen wir keine Reservierungen entgegen — bitte fragen Sie unser Personal.",
	"it":    "Non accettiamo prenotazioni da questa chat — chiedi al nostro personale.",
	"pt":    "Não aceitamos reservas por este chat — pergunte à nossa equipe.",
	"nl":    "We nemen via deze chat geen reserveringen aan — vraag het ons personeel.",
	"da":    "Vi tager ikke reservationer via denne chat — spørg personalet.",
	"no":    "Vi tar ikke reservasjoner via denne chatten — spør personalet.",
	"sv":    "Vi tar inte reservationer via den här chatten — fråga personalen.",
	"pl":    "Nie przyjmujemy rezerwacji na tym czacie — zapytaj personel.",
	"ru":    "Через этот чат брони не принимаем — уточните у персонала.",
	"tr":    "Bu sohbetten rezervasyon almıyoruz — lütfen personelimize sorun.",
	"ar":    "لا نأخذ حجوزات عبر هذه المحادثة — يرجى سؤال موظفينا.",
	"hi":    "इस चैट से बुकिंग नहीं लेते — कृपया स्टाफ से पूछें।",
	"ja":    "このチャットでは予約を受け付けていません — スタッフにお尋ねください。",
	"ko":    "이 채팅으로는 예약할 수 없습니다 — 직원에게 물어보세요.",
	"th":    "ไม่รับจองผ่านแชทนี้ — ถามพนักงานได้เลย",
	"vi":    "Chúng tôi không nhận đặt chỗ qua chat này — hãy hỏi nhân viên.",
	"zh":    "此聊天无法预订——请向工作人员询问。",
}

var billEmptyCopy = map[string]string{
	"en":    "There's no open check on this table yet.",
	"es":    "Todavía no hay una cuenta abierta en esta mesa.",
	"es-AR": "Todavía no hay una cuenta abierta en esta mesa.",
	"fr":    "Il n'y a pas encore d'addition ouverte à cette table.",
	"de":    "Für diesen Tisch gibt es noch keine offene Rechnung.",
	"it":    "Non c'è ancora un conto aperto a questo tavolo.",
	"pt":    "Ainda não há uma conta aberta nesta mesa.",
	"nl":    "Er is nog geen open rekening aan deze tafel.",
	"da":    "Der er endnu ingen åben regning ved dette bord.",
	"no":    "Det er ingen åpen regning ved dette bordet ennå.",
	"sv":    "Det finns ingen öppen nota vid det här bordet än.",
	"pl":    "Przy tym stoliku nie ma jeszcze otwartego rachunku.",
	"ru":    "На этом столе пока нет открытого счёта.",
	"tr":    "Bu masada henüz açık bir hesap yok.",
	"ar":    "لا توجد فاتورة مفتوحة على هذه الطاولة بعد.",
	"hi":    "इस टेबल पर अभी कोई खुला बिल नहीं है।",
	"ja":    "このテーブルにはまだ会計がありません。",
	"ko":    "이 테이블에는 아직 열린 계산서가 없습니다.",
	"th":    "โต๊ะนี้ยังไม่มีบิลเปิด",
	"vi":    "Bàn này chưa có hóa đơn mở.",
	"zh":    "这张桌子还没有未结账单。",
}

var billIntroCopy = map[string]string{
	"en":    "Here's your current check:",
	"es":    "Esta es tu cuenta actual:",
	"es-AR": "Esta es tu cuenta actual:",
	"fr":    "Voici votre addition actuelle :",
	"de":    "Hier ist Ihre aktuelle Rechnung:",
	"it":    "Ecco il tuo conto attuale:",
	"pt":    "Aqui está a sua conta atual:",
	"nl":    "Hier is je huidige rekening:",
	"da":    "Her er din aktuelle regning:",
	"no":    "Her er den gjeldende regningen:",
	"sv":    "Här är din aktuella nota:",
	"pl":    "Oto Twój bieżący rachunek:",
	"ru":    "Вот ваш текущий счёт:",
	"tr":    "Güncel hesabınız:",
	"ar":    "هذه فاتورتك الحالية:",
	"hi":    "यह आपका वर्तमान बिल है:",
	"ja":    "現在のお会計です:",
	"ko":    "현재 계산서입니다:",
	"th":    "นี่คือบิลปัจจุบันของคุณ:",
	"vi":    "Đây là hóa đơn hiện tại của bạn:",
	"zh":    "这是您当前的账单：",
}

var parkingUnknownCopy = map[string]string{
	"en":    "I don't have parking details here — please ask our staff.",
	"es":    "No tengo datos de estacionamiento aquí — por favor consulta con nuestro personal.",
	"es-AR": "No tengo datos de estacionamiento acá — consultá con nuestro personal.",
	"fr":    "Je n'ai pas les informations de stationnement — demandez à notre personnel.",
	"de":    "Ich habe hier keine Parkinformationen — bitte fragen Sie unser Personal.",
	"it":    "Non ho i dettagli sul parcheggio — chiedi al nostro personale.",
	"pt":    "Não tenho detalhes de estacionamento aqui — pergunte à nossa equipe.",
	"nl":    "Ik heb hier geen parkeerinfo — vraag het ons personeel.",
	"da":    "Jeg har ikke parkeringsoplysninger her — spørg personalet.",
	"no":    "Jeg har ikke parkeringsinfo her — spør personalet.",
	"sv":    "Jag har inte parkeringsinfo här — fråga personalen.",
	"pl":    "Nie mam tu informacji o parkingu — zapytaj personel.",
	"ru":    "У меня нет данных о парковке — уточните у персонала.",
	"tr":    "Park bilgisi burada yok — lütfen personelimize sorun.",
	"ar":    "ليست لدي تفاصيل الموقف هنا — يرجى سؤال موظفينا.",
	"hi":    "पार्किंग की जानकारी यहाँ नहीं है — कृपया स्टाफ से पूछें।",
	"ja":    "駐車場の情報はありません — スタッフにお尋ねください。",
	"ko":    "주차 정보가 여기 없습니다 — 직원에게 물어보세요.",
	"th":    "ฉันไม่มีข้อมูลที่จอดรถที่นี่ — ถามพนักงานได้เลย",
	"vi":    "Tôi không có thông tin gửi xe tại đây — hãy hỏi nhân viên.",
	"zh":    "这里没有停车信息——请向工作人员询问。",
}

var deliveryUnknownCopy = map[string]string{
	"en":    "I don't have delivery details here — please ask our staff.",
	"es":    "No tengo datos de delivery aquí — por favor consulta con nuestro personal.",
	"es-AR": "No tengo datos de delivery acá — consultá con nuestro personal.",
	"fr":    "Je n'ai pas les informations de livraison — demandez à notre personnel.",
	"de":    "Ich habe hier keine Lieferinformationen — bitte fragen Sie unser Personal.",
	"it":    "Non ho i dettagli sulla consegna — chiedi al nostro personale.",
	"pt":    "Não tenho detalhes de entrega aqui — pergunte à nossa equipe.",
	"nl":    "Ik heb hier geen bezorginfo — vraag het ons personeel.",
	"da":    "Jeg har ikke leveringsoplysninger her — spørg personalet.",
	"no":    "Jeg har ikke leveringsinfo her — spør personalet.",
	"sv":    "Jag har inte leveransinfo här — fråga personalen.",
	"pl":    "Nie mam tu informacji o dostawie — zapytaj personel.",
	"ru":    "У меня нет данных о доставке — уточните у персонала.",
	"tr":    "Teslimat bilgisi burada yok — lütfen personelimize sorun.",
	"ar":    "ليست لدي تفاصيل التوصيل هنا — يرجى سؤال موظفينا.",
	"hi":    "डिलीवरी की जानकारी यहाँ नहीं है — कृपया स्टाफ से पूछें।",
	"ja":    "配達の情報はありません — スタッフにお尋ねください。",
	"ko":    "배달 정보가 여기 없습니다 — 직원에게 물어보세요.",
	"th":    "ฉันไม่มีข้อมูลเดลิเวอรีที่นี่ — ถามพนักงานได้เลย",
	"vi":    "Tôi không có thông tin giao hàng tại đây — hãy hỏi nhân viên.",
	"zh":    "这里没有外卖信息——请向工作人员询问。",
}

var deliveryOffCopy = map[string]string{
	"en":    "We don't offer delivery right now.",
	"es":    "Por ahora no hacemos delivery.",
	"es-AR": "Por ahora no hacemos delivery.",
	"fr":    "Nous ne proposons pas la livraison pour le moment.",
	"de":    "Wir bieten derzeit keine Lieferung an.",
	"it":    "Al momento non facciamo consegne.",
	"pt":    "No momento não fazemos entrega.",
	"nl":    "We bezorgen op dit moment niet.",
	"da":    "Vi leverer ikke lige nu.",
	"no":    "Vi leverer ikke akkurat nå.",
	"sv":    "Vi levererar inte just nu.",
	"pl":    "Obecnie nie realizujemy dostaw.",
	"ru":    "Сейчас мы не доставляем.",
	"tr":    "Şu anda teslimat yapmıyoruz.",
	"ar":    "لا نوفر التوصيل حالياً.",
	"hi":    "अभी हम डिलीवरी नहीं करते।",
	"ja":    "現在デリバリーは行っておりません。",
	"ko":    "현재 배달은 하지 않습니다.",
	"th":    "ตอนนี้เรายังไม่มีบริการจัดส่ง",
	"vi":    "Hiện chúng tôi không giao hàng.",
	"zh":    "我们目前不提供外送。",
}

var deliveryInHouseCopy = map[string]string{
	"en":    "Yes — we deliver. The delivery fee is {fee}.",
	"es":    "Sí — hacemos delivery. El envío sale {fee}.",
	"es-AR": "Sí — hacemos delivery. El envío sale {fee}.",
	"fr":    "Oui — nous livrons. Les frais de livraison sont de {fee}.",
	"de":    "Ja — wir liefern. Die Liefergebühr beträgt {fee}.",
	"it":    "Sì — consegniamo. Il costo di consegna è {fee}.",
	"pt":    "Sim — fazemos entrega. A taxa de entrega é {fee}.",
	"nl":    "Ja — we bezorgen. De bezorgkosten zijn {fee}.",
	"da":    "Ja — vi leverer. Leveringsgebyret er {fee}.",
	"no":    "Ja — vi leverer. Leveringsgebyret er {fee}.",
	"sv":    "Ja — vi levererar. Leveransavgiften är {fee}.",
	"pl":    "Tak — dowozimy. Opłata za dostawę to {fee}.",
	"ru":    "Да — мы доставляем. Стоимость доставки {fee}.",
	"tr":    "Evet — teslimat yapıyoruz. Teslimat ücreti {fee}.",
	"ar":    "نعم — نوفر التوصيل. رسوم التوصيل {fee}.",
	"hi":    "हाँ — हम डिलीवरी करते हैं। डिलीवरी शुल्क {fee} है।",
	"ja":    "はい — 配達しています。配送料は {fee} です。",
	"ko":    "네 — 배달합니다. 배달료는 {fee}입니다.",
	"th":    "มีค่ะ — เราจัดส่ง ค่าจัดส่ง {fee}",
	"vi":    "Có — chúng tôi giao hàng. Phí giao hàng là {fee}.",
	"zh":    "是的——我们提供外送。配送费为 {fee}。",
}

var deliveryPartnersCopy = map[string]string{
	"en":    "Yes — we deliver through {partners}.",
	"es":    "Sí — hacemos delivery con {partners}.",
	"es-AR": "Sí — hacemos delivery con {partners}.",
	"fr":    "Oui — nous livrons via {partners}.",
	"de":    "Ja — wir liefern über {partners}.",
	"it":    "Sì — consegniamo tramite {partners}.",
	"pt":    "Sim — fazemos entrega por {partners}.",
	"nl":    "Ja — we bezorgen via {partners}.",
	"da":    "Ja — vi leverer via {partners}.",
	"no":    "Ja — vi leverer via {partners}.",
	"sv":    "Ja — vi levererar via {partners}.",
	"pl":    "Tak — dowozimy przez {partners}.",
	"ru":    "Да — мы доставляем через {partners}.",
	"tr":    "Evet — {partners} üzerinden teslimat yapıyoruz.",
	"ar":    "نعم — نوصل عبر {partners}.",
	"hi":    "हाँ — हम {partners} के ज़रिए डिलीवरी करते हैं।",
	"ja":    "はい — {partners} で配達しています。",
	"ko":    "네 — {partners}로 배달합니다.",
	"th":    "มีค่ะ — เราจัดส่งผ่าน {partners}",
	"vi":    "Có — chúng tôi giao qua {partners}.",
	"zh":    "是的——我们通过 {partners} 外送。",
}

var deliveryBothCopy = map[string]string{
	"en":    "Yes — we deliver ourselves for {fee}, and you can also order through {partners}.",
	"es":    "Sí — hacemos delivery propio por {fee}, y también puedes pedir por {partners}.",
	"es-AR": "Sí — hacemos delivery propio por {fee}, y también podés pedir por {partners}.",
	"fr":    "Oui — nous livrons nous-mêmes pour {fee}, et vous pouvez aussi commander via {partners}.",
	"de":    "Ja — wir liefern selbst für {fee}, und Sie können auch über {partners} bestellen.",
	"it":    "Sì — consegniamo noi a {fee}, e puoi ordinare anche tramite {partners}.",
	"pt":    "Sim — fazemos entrega própria por {fee}, e você também pode pedir por {partners}.",
	"nl":    "Ja — we bezorgen zelf voor {fee}, en je kunt ook bestellen via {partners}.",
	"da":    "Ja — vi leverer selv for {fee}, og du kan også bestille via {partners}.",
	"no":    "Ja — vi leverer selv for {fee}, og du kan også bestille via {partners}.",
	"sv":    "Ja — vi levererar själva för {fee}, och du kan även beställa via {partners}.",
	"pl":    "Tak — dowozimy sami za {fee}, możesz też zamówić przez {partners}.",
	"ru":    "Да — мы доставляем сами за {fee}, также можно заказать через {partners}.",
	"tr":    "Evet — {fee} karşılığında kendimiz teslim ediyoruz, ayrıca {partners} üzerinden de sipariş verebilirsiniz.",
	"ar":    "نعم — نوصل بأنفسنا مقابل {fee}، ويمكنك أيضاً الطلب عبر {partners}.",
	"hi":    "हाँ — हम {fee} में खुद डिलीवरी करते हैं, और आप {partners} से भी ऑर्डर कर सकते हैं।",
	"ja":    "はい — 当店の配送は {fee} で、{partners} からもご注文いただけます。",
	"ko":    "네 — 직접 배달은 {fee}이며, {partners}로도 주문하실 수 있습니다.",
	"th":    "มีค่ะ — เราจัดส่งเองในราคา {fee} และสั่งผ่าน {partners} ได้เช่นกัน",
	"vi":    "Có — chúng tôi tự giao với phí {fee}, bạn cũng có thể đặt qua {partners}.",
	"zh":    "是的——我们自送收费 {fee}，也可以通过 {partners} 下单。",
}

var tablesUnknownCopy = map[string]string{
	"en":    "I can't see live table occupancy from this chat.",
	"es":    "No puedo ver las mesas libres en vivo desde este chat.",
	"es-AR": "No puedo ver las mesas libres en vivo desde este chat.",
	"fr":    "Je ne vois pas l'occupation des tables en direct depuis ce chat.",
	"de":    "Ich sehe die Tischbelegung in diesem Chat nicht live.",
	"it":    "Da questa chat non vedo i tavoli liberi in tempo reale.",
	"pt":    "Não consigo ver as mesas livres ao vivo neste chat.",
	"nl":    "Ik zie de vrije tafels niet live in deze chat.",
	"da":    "Jeg kan ikke se ledige borde live i denne chat.",
	"no":    "Jeg kan ikke se ledige bord live i denne chatten.",
	"sv":    "Jag kan inte se lediga bord live i den här chatten.",
	"pl":    "Nie widzę wolnych stolików na żywo z tego czatu.",
	"ru":    "Из этого чата не видно свободные столы.",
	"tr":    "Bu sohbetten canlı masa durumunu göremiyorum.",
	"ar":    "لا أستطيع رؤية الطاولات المتاحة مباشرة من هذه المحادثة.",
	"hi":    "इस चैट से लाइव टेबल नहीं दिखतीं।",
	"ja":    "このチャットでは空席をリアルタイムでは確認できません。",
	"ko":    "이 채팅에서는 실시간 테이블 현황을 볼 수 없습니다.",
	"th":    "ฉันดูโต๊ะว่างแบบสดจากแชทนี้ไม่ได้",
	"vi":    "Tôi không thấy bàn trống trực tiếp từ chat này.",
	"zh":    "这个聊天里看不到实时空桌。",
}

var serviceCallCopy = map[string]string{
	"en":    "I can't ping the floor from this chat — use Call waiter on the table page.",
	"es":    "No puedo avisar al salón desde este chat — usá Llamar al mozo en la página de la mesa.",
	"es-AR": "No puedo avisar al salón desde este chat — usá Llamar al mozo en la página de la mesa.",
	"fr":    "Je ne peux pas appeler la salle depuis ce chat — utilisez Appeler un serveur sur la page de la table.",
	"de":    "Ich kann den Service aus diesem Chat nicht rufen — nutzen Sie Kellner rufen auf der Tischseite.",
	"it":    "Non posso avvisare la sala da questa chat — usa Chiama il cameriere nella pagina del tavolo.",
	"pt":    "Não consigo chamar o salão por este chat — use Chamar garçom na página da mesa.",
	"nl":    "Ik kan de bediening niet bereiken vanuit deze chat — gebruik Roep ober op de tafelpagina.",
	"da":    "Jeg kan ikke kalde personalet fra denne chat — brug Kald tjener på bordsiden.",
	"no":    "Jeg kan ikke kalle på personalet fra denne chatten — bruk Kall på servitør på bordsiden.",
	"sv":    "Jag kan inte ropa på golvet från den här chatten — använd Kalla på servitör på bordsidan.",
	"pl":    "Nie mogę wezwać obsługi z tego czatu — użyj Wezwij kelnera na stronie stolika.",
	"ru":    "Из этого чата я не могу позвать официанта — нажмите Позвать официанта на странице стола.",
	"tr":    "Bu sohbetten salonu çağıramam — masa sayfasındaki Garson çağır seçeneğini kullanın.",
	"ar":    "لا أستطيع تنبيه الصالة من هذه المحادثة — استخدم اطلب النادل في صفحة الطاولة.",
	"hi":    "इस चैट से फ्लोर नहीं बुला सकते — टेबल पेज पर वेटर बुलाएँ।",
	"ja":    "このチャットからホールに連絡できません — テーブルページの「スタッフを呼ぶ」をご利用ください。",
	"ko":    "이 채팅으로는 홀에 연락할 수 없습니다 — 테이블 페이지의 직원 호출을 이용하세요.",
	"th":    "ฉันเรียกพนักงานจากแชทนี้ไม่ได้ — ใช้ เรียกพนักงาน ในหน้าโต๊ะ",
	"vi":    "Tôi không gọi được nhân viên từ chat này — dùng Gọi phục vụ trên trang bàn.",
	"zh":    "这个聊天无法呼叫现场——请使用桌位页的呼叫服务员。",
}

// waitTimeCopy answers "how long it takes?" (issue 790b). No house prep-time
// setting exists in the product, so the copy routes to a kitchen check via
// staff instead of fabricating minutes.
var waitTimeCopy = map[string]string{
	"en":    "I can't see live kitchen timing from this chat — our staff can check with the kitchen for you; use Call waiter on the table page.",
	"es":    "No puedo ver los tiempos de la cocina desde este chat — nuestro personal puede consultarlo con la cocina; usa Llamar al camarero en la página de la mesa.",
	"es-AR": "No puedo ver los tiempos de la cocina desde este chat — nuestro personal puede consultarlo con la cocina; usá Llamar al mozo en la página de la mesa.",
	"fr":    "Je ne vois pas les délais de la cuisine depuis ce chat — notre personnel peut vérifier auprès de la cuisine ; utilisez Appeler un serveur sur la page de la table.",
	"de":    "Ich sehe die Küchenzeiten in diesem Chat nicht — unser Personal kann in der Küche nachfragen; nutzen Sie Kellner rufen auf der Tischseite.",
	"it":    "Non vedo i tempi della cucina da questa chat — il nostro staff può chiedere alla cucina; usa Chiama il cameriere nella pagina del tavolo.",
	"pt":    "Não vejo os tempos da cozinha por este chat — nossa equipe pode verificar com a cozinha; use Chamar garçom na página da mesa.",
	"nl":    "Ik zie de keukentijden niet in deze chat — ons personeel kan het bij de keuken navragen; gebruik Roep ober op de tafelpagina.",
	"da":    "Jeg kan ikke se køkkenets tider fra denne chat — personalet kan tjekke med køkkenet; brug Kald tjener på bordsiden.",
	"no":    "Jeg kan ikke se kjøkkenets tider fra denne chatten — personalet kan sjekke med kjøkkenet; bruk Kall på servitør på bordsiden.",
	"sv":    "Jag kan inte se kökets tider från den här chatten — personalen kan kolla med köket; använd Kalla på servitör på bordsidan.",
	"pl":    "Nie widzę czasów kuchni z tego czatu — obsługa może sprawdzić w kuchni; użyj Wezwij kelnera na stronie stolika.",
	"ru":    "Из этого чата не видно, сколько готовит кухня — персонал может уточнить на кухне; нажмите Позвать официанта на странице стола.",
	"tr":    "Bu sohbetten mutfağın süresini göremiyorum — personelimiz mutfağa sorabilir; masa sayfasındaki Garson çağır seçeneğini kullanın.",
	"ar":    "لا أستطيع رؤية توقيت المطبخ من هذه المحادثة — يمكن لموظفينا السؤال في المطبخ؛ استخدم اطلب النادل في صفحة الطاولة.",
	"hi":    "इस चैट से किचन का समय नहीं दिखता — स्टाफ किचन से पता कर सकता है; टेबल पेज पर वेटर बुलाएँ।",
	"ja":    "このチャットではキッチンの状況が確認できません — スタッフが厨房に確認できます。テーブルページの「スタッフを呼ぶ」をご利用ください。",
	"ko":    "이 채팅으로는 주방 시간을 알 수 없습니다 — 직원이 주방에 확인해 드릴 수 있어요; 테이블 페이지의 직원 호출을 이용하세요.",
	"th":    "ฉันดูเวลาในครัวจากแชทนี้ไม่ได้ — พนักงานสามารถเช็คกับครัวให้ได้ ใช้ เรียกพนักงาน ในหน้าโต๊ะ",
	"vi":    "Tôi không thấy thời gian bếp từ chat này — nhân viên có thể hỏi bếp giúp bạn; dùng Gọi phục vụ trên trang bàn.",
	"zh":    "这个聊天里看不到后厨的出餐时间——工作人员可以帮您向厨房确认；请使用桌位页的呼叫服务员。",
}

var soldOutIntroCopy = map[string]string{
	"en":    "These are sold out right now:",
	"es":    "Esto está agotado ahora:",
	"es-AR": "Esto está agotado ahora:",
	"fr":    "Voici ce qui est épuisé en ce moment :",
	"de":    "Das ist gerade ausverkauft:",
	"it":    "Questi sono esauriti adesso:",
	"pt":    "Isto está esgotado agora:",
	"nl":    "Dit is nu uitverkocht:",
	"da":    "Disse er udsolgt lige nu:",
	"no":    "Disse er utsolgt akkurat nå:",
	"sv":    "Dessa är slutsålda just nu:",
	"pl":    "To jest teraz wyprzedane:",
	"ru":    "Сейчас распродано:",
	"tr":    "Şu anda tükenenler:",
	"ar":    "هذه غير متاحة الآن:",
	"hi":    "अभी ये बिक चुके हैं:",
	"ja":    "ただいま売り切れです:",
	"ko":    "지금 품절입니다:",
	"th":    "ตอนนี้หมดแล้ว:",
	"vi":    "Những món này hiện hết hàng:",
	"zh":    "目前已售罄：",
}

var soldOutNoneCopy = map[string]string{
	"en":    "Nothing on the menu is marked sold out right now.",
	"es":    "Ahora mismo no hay nada del menú marcado como agotado.",
	"es-AR": "Ahora mismo no hay nada del menú marcado como agotado.",
	"fr":    "Rien au menu n'est marqué épuisé pour le moment.",
	"de":    "Gerade ist nichts auf der Karte als ausverkauft markiert.",
	"it":    "Al momento nulla nel menu è segnato come esaurito.",
	"pt":    "Nada no cardápio está marcado como esgotado agora.",
	"nl":    "Er staat nu niets op het menu als uitverkocht.",
	"da":    "Intet på menuen er markeret som udsolgt lige nu.",
	"no":    "Ingenting på menyen er merket utsolgt akkurat nå.",
	"sv":    "Inget på menyn är markerat som slutsålt just nu.",
	"pl":    "Nic w menu nie jest teraz oznaczone jako wyprzedane.",
	"ru":    "Сейчас в меню ничего не отмечено как распродано.",
	"tr":    "Menüde şu anda tükenen bir şey işaretli değil.",
	"ar":    "لا شيء في القائمة مميز كنافد حالياً.",
	"hi":    "मेनू पर अभी कुछ भी सोल्ड आउट नहीं है।",
	"ja":    "現在メニューに売り切れ表示はありません。",
	"ko":    "지금 메뉴에 품절로 표시된 항목은 없습니다.",
	"th":    "ตอนนี้ไม่มีรายการในเมนูที่ทำเครื่องหมายว่าหมด",
	"vi":    "Hiện không có món nào trên thực đơn bị đánh dấu hết hàng.",
	"zh":    "目前菜单上没有标为售罄的菜品。",
}

var offMenuNamedCopy = map[string]string{
	"en":    waiterVisitPlaceholder + " isn't on our menu.",
	"es":    waiterVisitPlaceholder + " no está en el menú.",
	"es-AR": waiterVisitPlaceholder + " no está en el menú.",
	"fr":    waiterVisitPlaceholder + " n'est pas au menu.",
	"de":    waiterVisitPlaceholder + " steht nicht auf der Speisekarte.",
	"it":    waiterVisitPlaceholder + " non è nel menu.",
	"pt":    waiterVisitPlaceholder + " não está no cardápio.",
	"nl":    waiterVisitPlaceholder + " staat niet op het menu.",
	"da":    waiterVisitPlaceholder + " er ikke på menuen.",
	"no":    waiterVisitPlaceholder + " er ikke på menyen.",
	"sv":    waiterVisitPlaceholder + " finns inte på menyn.",
	"pl":    waiterVisitPlaceholder + " nie ma w menu.",
	"ru":    waiterVisitPlaceholder + " нет в меню.",
	"tr":    waiterVisitPlaceholder + " menüde yok.",
	"ar":    waiterVisitPlaceholder + " ليس في القائمة.",
	"hi":    waiterVisitPlaceholder + " हमारे मेनू पर नहीं है।",
	"ja":    waiterVisitPlaceholder + " はメニューにありません。",
	"ko":    waiterVisitPlaceholder + " 은(는) 메뉴에 없습니다.",
	"th":    waiterVisitPlaceholder + " ไม่อยู่ในเมนู",
	"vi":    waiterVisitPlaceholder + " không có trong thực đơn.",
	"zh":    waiterVisitPlaceholder + " 不在菜单上。",
}

func WaiterHoursAnswer(locale string, facts WaiterVisitFacts) string {
	if !facts.HoursKnown {
		return waiterVisitCopy(hoursUnknownCopy, locale)
	}
	if facts.TodayClosed {
		return waiterVisitCopy(hoursClosedCopy, locale)
	}
	if waiterHoursAllDay(facts.TodayOpen, facts.TodayClose) {
		return waiterVisitCopy(hoursOpenAllDayCopy, locale)
	}
	out := waiterVisitCopy(hoursOpenCopy, locale)
	out = strings.ReplaceAll(out, "{open}", facts.TodayOpen)
	out = strings.ReplaceAll(out, "{close}", facts.TodayClose)
	return out
}

// waiterHoursAllDay reports whether an open/close pair is the "always open"
// window: equal clock times (00:00-00:00, or 09:00-09:00 for a venue whose day
// rolls over at nine). The comparison is on parsed minutes so "00:00" and
// "00:00:00" (a TIME column read back with seconds) still match.
func waiterHoursAllDay(open, closeAt string) bool {
	openMin, okOpen := directorClockMinutes(open)
	closeMin, okClose := directorClockMinutes(closeAt)
	if !okOpen || !okClose {
		return false
	}
	return openMin%(24*60) == closeMin%(24*60)
}

func WaiterReservationAnswer(locale string, facts WaiterVisitFacts) string {
	if !facts.Reservations {
		return waiterVisitCopy(reservationDisabledCopy, locale)
	}
	out := waiterVisitCopy(reservationEnabledCopy, locale)
	out = strings.ReplaceAll(out, "{min}", fmt.Sprintf("%d", facts.PartyMin))
	out = strings.ReplaceAll(out, "{max}", fmt.Sprintf("%d", facts.PartyMax))
	return out
}

func WaiterBillAnswer(locale string, facts WaiterVisitFacts) string {
	if !facts.HasOpenBill || strings.TrimSpace(facts.BillSummary) == "" {
		return waiterVisitCopy(billEmptyCopy, locale)
	}
	return waiterVisitCopy(billIntroCopy, locale) + "\n" + strings.TrimSpace(facts.BillSummary)
}

func WaiterParkingAnswer(locale string) string {
	return waiterVisitCopy(parkingUnknownCopy, locale)
}

// WaiterDeliveryAnswer answers "do you deliver?" from the venue's real delivery
// configuration, in the guest's language.
//
// It used to return WaiterVisitFacts.DeliveryHint verbatim — the model's prompt
// sentence. That shipped English operator instructions ("Direct guests to the
// delivery section on this page") to guests in all 21 locales, printed the fee
// as a bare unlabelled number, never named the marketplaces the operator had
// configured, and fell through to "I don't have delivery details here" for a
// venue whose Delivery settings already list PedidosYa and Rappi (#903).
func WaiterDeliveryAnswer(locale string, facts WaiterVisitFacts) string {
	if !facts.DeliveryKnown {
		return waiterVisitCopy(deliveryUnknownCopy, locale)
	}
	if !facts.DeliveryEnabled {
		return waiterVisitCopy(deliveryOffCopy, locale)
	}

	fee := strings.TrimSpace(facts.DeliveryFeeLabel)
	inHouse := facts.DeliveryInHouse && fee != ""
	partners := waiterDeliveryPartnerList(facts.DeliveryPartners)

	switch {
	case inHouse && partners != "":
		answer := waiterVisitCopy(deliveryBothCopy, locale)
		answer = strings.ReplaceAll(answer, "{fee}", fee)
		return strings.ReplaceAll(answer, "{partners}", partners)
	case inHouse:
		return strings.ReplaceAll(waiterVisitCopy(deliveryInHouseCopy, locale), "{fee}", fee)
	case partners != "":
		return strings.ReplaceAll(waiterVisitCopy(deliveryPartnersCopy, locale), "{partners}", partners)
	}

	// Delivery is switched on but nothing usable is configured (no in-house fee,
	// no partner). Claiming "yes" here would promise a channel the guest cannot
	// find, so defer to staff rather than invent one.
	return waiterVisitCopy(deliveryUnknownCopy, locale)
}

// WaiterDeliveryPartnerNames lists the marketplaces this venue already has
// configured — the operator's external partner links unioned with the built-in
// Uber Eats / DoorDash / Grubhub toggles. It shares the Director's partner
// reader (#871) so the host and the guest storefront can never disagree about
// who delivers for this venue.
func WaiterDeliveryPartnerNames(settings *database.DeliverySettings) []string {
	if settings == nil {
		return nil
	}
	return directorDeliveryPartners(settings.ExternalPartnerLinks,
		settings.UberEatsEnabled, settings.DoordashEnabled, settings.GrubhubEnabled)
}

// waiterDeliveryPartnerList renders the configured marketplace names. Blank
// entries are dropped so a half-filled partner row never prints an empty slot.
func waiterDeliveryPartnerList(partners []string) string {
	named := make([]string, 0, len(partners))
	for _, partner := range partners {
		if trimmed := strings.TrimSpace(partner); trimmed != "" {
			named = append(named, trimmed)
		}
	}
	return strings.Join(named, ", ")
}

func WaiterTablesAnswer(locale string) string {
	return waiterVisitCopy(tablesUnknownCopy, locale)
}

func WaiterServiceCallAnswer(locale string) string {
	return waiterVisitCopy(serviceCallCopy, locale)
}

func WaiterWaitTimeAnswer(locale string) string {
	return waiterVisitCopy(waitTimeCopy, locale)
}

func WaiterSoldOutIntro(locale string) string {
	return waiterVisitCopy(soldOutIntroCopy, locale)
}

func WaiterSoldOutNone(locale string) string {
	return waiterVisitCopy(soldOutNoneCopy, locale)
}

func WaiterOffMenuNamed(locale, name string) string {
	return waiterVisitWithItem(waiterVisitCopy(offMenuNamedCopy, locale), name)
}

// WaiterHoursPeriod is one open service window from today's operating rows.
type WaiterHoursPeriod struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// BuildWaiterHoursPeriods reads ALL of today's operating windows in the business
// timezone, in the order the rows arrive (GetBusinessOperatingHoursByDay orders
// by open_time).
//
// ValidateBusinessOperatingHours permits two rows per day, so a split-shift
// venue running lunch and dinner has two windows. A caller that stops at the
// first row tells a mid-afternoon owner the venue shut for the day at 15:00 and
// an owner at 20:30 that the venue is closed while dinner service is running.
//
// closed is true only when today has rows and none of them is a usable open
// window; known is false when today has no rows at all ("we do not know" and
// "we are closed" are different answers).
func BuildWaiterHoursPeriods(hours []database.BusinessOperatingHours, timezone string, now time.Time) (periods []WaiterHoursPeriod, closed, known bool) {
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || strings.TrimSpace(timezone) == "" {
		loc = time.UTC
	}
	weekday := int(now.In(loc).Weekday())
	var today []database.BusinessOperatingHours
	for _, row := range hours {
		if row.DayOfWeek == weekday {
			today = append(today, row)
		}
	}
	if len(today) == 0 {
		return nil, false, false
	}
	for _, row := range today {
		if row.IsClosed {
			continue
		}
		openTime := strings.TrimSpace(row.OpenTime)
		closeTime := strings.TrimSpace(row.CloseTime)
		if openTime == "" || closeTime == "" {
			continue
		}
		periods = append(periods, WaiterHoursPeriod{Open: openTime, Close: closeTime})
	}
	if len(periods) == 0 {
		return nil, true, true
	}
	return periods, false, true
}

// BuildWaiterHoursFacts reads today's FIRST operating window in the business
// timezone. An equal open and close (00:00-00:00) is returned as-is: it means
// open all 24 hours, and WaiterHoursAnswer words it that way. Kept for the guest concierge, whose single-window copy line exists
// in all 21 guest locales; callers that must not drop a split shift (the
// Director's venue clock) read BuildWaiterHoursPeriods instead.
func BuildWaiterHoursFacts(hours []database.BusinessOperatingHours, timezone string, now time.Time) (open, close string, closed, known bool) {
	periods, closedToday, hoursKnown := BuildWaiterHoursPeriods(hours, timezone, now)
	if !hoursKnown || closedToday || len(periods) == 0 {
		return "", "", closedToday, hoursKnown
	}
	return periods[0].Open, periods[0].Close, false, true
}
