/**
 * Provider-free copy for route-level error / not-found boundaries.
 *
 * Error boundaries must not depend on GuestTranslationProvider (or any other
 * i18n context) — that tree may already have crashed. Resolve language from
 * `?lang=`, `<html lang>`, or `navigator.language` and pick a static map entry.
 */

import type { StorefrontLocale } from "./localeRegistry";

export type BoundaryCopy = {
  title: string;
  body: string;
  /** Table error body with reassurance that the table/order is safe. */
  tableBody: string;
  retry: string;
  back: string;
  home: string;
  notFound: string;
  backToTable: string;
  backToRestaurant: string;
  /** Label prefix before the digest, e.g. "Error code". */
  errorCode: string;
  profileHeading: string;
  profileBody: string;
  profileBack: string;
  toolsHeading: string;
  toolsBody: string;
  toolsBack: string;
  notFoundTitle: string;
  notFoundBody: string;
  /** Table-code 404 heading — never reuse storefront custom-URL copy. */
  tableNotFoundTitle: string;
  /** Table-code 404 body for dead / rotated QR codes. */
  tableNotFoundBody: string;
  goHome: string;
  scanLabel: string;
  scanTitle: string;
  scanBody: string;
  scanAgain: string;
  reservationBody: string;
  backToReservation: string;
  deliveryBody: string;
  backToTracking: string;
};

const COPY: Record<string, BoundaryCopy> = {
  en: {
    title: "Something went wrong",
    body: "Sorry about that — this is on us, not you.",
    tableBody:
      "Sorry about that — this is on us, not you. Your table and any order you placed are safe.",
    retry: "Try again",
    back: "Back",
    home: "Home",
    notFound: "Not found",
    backToTable: "Back to your table",
    backToRestaurant: "Back to the restaurant page",
    errorCode: "Error code",
    profileHeading: "Couldn't load your profile.",
    profileBody: "Try again or sign back in.",
    profileBack: "Back to profile",
    toolsHeading: "Couldn't load the tool.",
    toolsBody: "Try again, or pick a different tool from the menu.",
    toolsBack: "Back to tools",
    notFoundTitle: "We couldn't find that page",
    notFoundBody:
      "We couldn't find a business with that custom URL. It may have been removed, or the URL may be misspelled.",
    tableNotFoundTitle: "We couldn't find that table",
    tableNotFoundBody:
      "That table code isn't active. The QR code may be old, or the table may have been reset. Ask staff for a current code.",
    goHome: "Go to homepage",
    scanLabel: "Scan error",
    scanTitle: "We couldn't open the scanner.",
    scanBody: "Try again in a moment, or ask your server for the table code.",
    scanAgain: "Scan again",
    reservationBody: "Sorry about that — your reservation is unaffected.",
    backToReservation: "Back to your reservation",
    deliveryBody: "Sorry about that — your delivery order is unaffected.",
    backToTracking: "Back to order tracking",
  },
  es: {
    title: "Algo salió mal",
    body: "Lo sentimos — es culpa nuestra, no tuya.",
    tableBody:
      "Lo sentimos — es culpa nuestra, no tuya. Tu mesa y cualquier pedido que hayas hecho están a salvo.",
    retry: "Reintentar",
    back: "Volver",
    home: "Inicio",
    notFound: "No encontrado",
    backToTable: "Volver a tu mesa",
    backToRestaurant: "Volver a la página del restaurante",
    errorCode: "Código de error",
    profileHeading: "No pudimos cargar tu perfil.",
    profileBody: "Inténtalo de nuevo o vuelve a iniciar sesión.",
    profileBack: "Volver al perfil",
    toolsHeading: "No pudimos cargar la herramienta.",
    toolsBody: "Inténtalo de nuevo o elige otra herramienta del menú.",
    toolsBack: "Volver a herramientas",
    notFoundTitle: "No pudimos encontrar esa página",
    notFoundBody:
      "No encontramos un negocio con esa URL personalizada. Puede que se haya eliminado o que la URL esté mal escrita.",
    tableNotFoundTitle: "No encontramos esa mesa",
    tableNotFoundBody:
      "Ese código de mesa no está activo. El QR puede ser antiguo o la mesa se reinició. Pide al personal un código actual.",
    goHome: "Ir al inicio",
    scanLabel: "Error de escaneo",
    scanTitle: "No pudimos abrir el escáner.",
    scanBody:
      "Inténtalo de nuevo en un momento o pide el código de mesa a tu mesero.",
    scanAgain: "Escanear de nuevo",
    reservationBody: "Lo sentimos — tu reserva no se ha visto afectada.",
    backToReservation: "Volver a tu reserva",
    deliveryBody:
      "Lo sentimos — tu pedido a domicilio no se ha visto afectado.",
    backToTracking: "Volver al seguimiento del pedido",
  },
  "es-AR": {
    title: "Algo salió mal",
    body: "Disculpá — es culpa nuestra, no tuya.",
    tableBody:
      "Disculpá — es culpa nuestra, no tuya. Tu mesa y cualquier pedido que hayas hecho están a salvo.",
    retry: "Reintentar",
    back: "Volver",
    home: "Inicio",
    notFound: "No encontrado",
    backToTable: "Volver a tu mesa",
    backToRestaurant: "Volver a la página del restaurante",
    errorCode: "Código de error",
    profileHeading: "No pudimos cargar tu perfil.",
    profileBody: "Probá de nuevo o volvé a iniciar sesión.",
    profileBack: "Volver al perfil",
    toolsHeading: "No pudimos cargar la herramienta.",
    toolsBody: "Probá de nuevo o elegí otra herramienta del menú.",
    toolsBack: "Volver a herramientas",
    notFoundTitle: "No pudimos encontrar esa página",
    notFoundBody:
      "No encontramos un negocio con esa URL personalizada. Puede que se haya eliminado o que la URL esté mal escrita.",
    tableNotFoundTitle: "No encontramos esa mesa",
    tableNotFoundBody:
      "Ese código de mesa no está activo. El QR puede ser viejo o la mesa se reinició. Pedile al personal un código actual.",
    goHome: "Ir al inicio",
    scanLabel: "Error de escaneo",
    scanTitle: "No pudimos abrir el escáner.",
    scanBody:
      "Probá de nuevo en un momento o pedile el código de mesa a tu mozo.",
    scanAgain: "Escaneá de nuevo",
    reservationBody: "Disculpá — tu reserva no se ha visto afectada.",
    backToReservation: "Volver a tu reserva",
    deliveryBody: "Disculpá — tu pedido a domicilio no se ha visto afectado.",
    backToTracking: "Volver al seguimiento del pedido",
  },
  fr: {
    title: "Une erreur s'est produite",
    body: "Désolé — c'est de notre côté, pas du vôtre.",
    tableBody:
      "Désolé — c'est de notre côté, pas du vôtre. Votre table et toute commande passée sont en sécurité.",
    retry: "Réessayer",
    back: "Retour",
    home: "Accueil",
    notFound: "Introuvable",
    backToTable: "Retour à votre table",
    backToRestaurant: "Retour à la page du restaurant",
    errorCode: "Code d'erreur",
    profileHeading: "Impossible de charger votre profil.",
    profileBody: "Réessayez ou reconnectez-vous.",
    profileBack: "Retour au profil",
    toolsHeading: "Impossible de charger l'outil.",
    toolsBody: "Réessayez, ou choisissez un autre outil dans le menu.",
    toolsBack: "Retour aux outils",
    notFoundTitle: "Page introuvable",
    notFoundBody:
      "Nous n'avons pas trouvé d'établissement avec cette URL personnalisée. Il a peut-être été supprimé ou l'URL est mal orthographiée.",
    tableNotFoundTitle: "Nous n'avons pas trouvé cette table",
    tableNotFoundBody:
      "Ce code de table n'est pas actif. Le QR est peut-être ancien, ou la table a été réinitialisée. Demandez un code actuel au personnel.",
    goHome: "Aller à l'accueil",
    scanLabel: "Erreur de scan",
    scanTitle: "Nous n'avons pas pu ouvrir le scanner.",
    scanBody:
      "Réessayez dans un instant, ou demandez le code de table à votre serveur.",
    scanAgain: "Scanner à nouveau",
    reservationBody: "Désolé — votre réservation n'est pas affectée.",
    backToReservation: "Retour à votre réservation",
    deliveryBody: "Désolé — votre commande de livraison n'est pas affectée.",
    backToTracking: "Retour au suivi de commande",
  },
  de: {
    title: "Etwas ist schiefgelaufen",
    body: "Das tut uns leid — der Fehler liegt bei uns, nicht bei Ihnen.",
    tableBody:
      "Das tut uns leid — der Fehler liegt bei uns, nicht bei Ihnen. Ihr Tisch und jede aufgegebene Bestellung sind sicher.",
    retry: "Erneut versuchen",
    back: "Zurück",
    home: "Startseite",
    notFound: "Nicht gefunden",
    backToTable: "Zurück zu Ihrem Tisch",
    backToRestaurant: "Zurück zur Restaurantseite",
    errorCode: "Fehlercode",
    profileHeading: "Ihr Profil konnte nicht geladen werden.",
    profileBody: "Versuchen Sie es erneut oder melden Sie sich wieder an.",
    profileBack: "Zurück zum Profil",
    toolsHeading: "Das Tool konnte nicht geladen werden.",
    toolsBody:
      "Versuchen Sie es erneut oder wählen Sie ein anderes Tool im Menü.",
    toolsBack: "Zurück zu den Tools",
    notFoundTitle: "Wir konnten diese Seite nicht finden",
    notFoundBody:
      "Wir konnten kein Unternehmen mit dieser benutzerdefinierten URL finden. Möglicherweise wurde es entfernt oder die URL ist falsch geschrieben.",
    tableNotFoundTitle: "Wir konnten diesen Tisch nicht finden",
    tableNotFoundBody:
      "Dieser Tischcode ist nicht aktiv. Der QR-Code ist möglicherweise alt oder der Tisch wurde zurückgesetzt. Fragen Sie das Personal nach einem aktuellen Code.",
    goHome: "Zur Startseite",
    scanLabel: "Scan-Fehler",
    scanTitle: "Wir konnten den Scanner nicht öffnen.",
    scanBody:
      "Versuchen Sie es in einem Moment erneut, oder fragen Sie Ihr Servicepersonal nach dem Tischcode.",
    scanAgain: "Erneut scannen",
    reservationBody:
      "Das tut uns leid — Ihre Reservierung ist nicht betroffen.",
    backToReservation: "Zurück zu Ihrer Reservierung",
    deliveryBody:
      "Das tut uns leid — Ihre Lieferbestellung ist nicht betroffen.",
    backToTracking: "Zurück zur Auftragverfolgung",
  },
  ar: {
    title: "حدث خطأ ما",
    body: "نعتذر — المشكلة من جانبنا، وليس من جانبك.",
    tableBody:
      "نعتذر — المشكلة من جانبنا، وليس من جانبك. طاولتك وأي طلب قدّمته بأمان.",
    retry: "حاول مرة أخرى",
    back: "رجوع",
    home: "الرئيسية",
    notFound: "غير موجود",
    backToTable: "العودة إلى طاولتك",
    backToRestaurant: "العودة إلى صفحة المطعم",
    errorCode: "رمز الخطأ",
    profileHeading: "تعذّر تحميل ملفك الشخصي.",
    profileBody: "حاول مرة أخرى أو سجّل الدخول من جديد.",
    profileBack: "العودة إلى الملف الشخصي",
    toolsHeading: "تعذّر تحميل الأداة.",
    toolsBody: "حاول مرة أخرى، أو اختر أداة أخرى من القائمة.",
    toolsBack: "العودة إلى الأدوات",
    notFoundTitle: "لم نتمكن من العثور على هذه الصفحة",
    notFoundBody:
      "لم نتمكن من العثور على نشاط تجاري بهذا الرابط المخصص. ربما تمت إزالته أو أن الرابط مكتوب بشكل خاطئ.",
    tableNotFoundTitle: "لم نعثر على تلك الطاولة",
    tableNotFoundBody:
      "رمز الطاولة هذا غير نشط. قد يكون رمز QR قديماً أو أُعيد ضبط الطاولة. اطلب من الموظفين رمزاً حديثاً.",
    goHome: "الذهاب إلى الصفحة الرئيسية",
    scanLabel: "خطأ في المسح",
    scanTitle: "تعذّر فتح الماسح.",
    scanBody: "حاول مرة أخرى بعد لحظة، أو اطلب رمز الطاولة من النادل.",
    scanAgain: "امسح مرة أخرى",
    reservationBody: "نعتذر — حجزك لم يتأثر.",
    backToReservation: "العودة إلى حجزك",
    deliveryBody: "نعتذر — طلب التوصيل الخاص بك لم يتأثر.",
    backToTracking: "العودة إلى تتبع الطلب",
  },
  pt: {
    title: "Algo deu errado",
    body: "Desculpe — o problema é nosso, não seu.",
    tableBody:
      "Desculpe — o problema é nosso, não seu. Sua mesa e qualquer pedido feito estão seguros.",
    retry: "Tentar novamente",
    back: "Voltar",
    home: "Início",
    notFound: "Não encontrada",
    backToTable: "Voltar à sua mesa",
    backToRestaurant: "Voltar à página do restaurante",
    errorCode: "Código de erro",
    profileHeading: "Não foi possível carregar seu perfil.",
    profileBody: "Tente novamente ou entre de novo.",
    profileBack: "Voltar ao perfil",
    toolsHeading: "Não foi possível carregar a ferramenta.",
    toolsBody: "Tente novamente ou escolha outra ferramenta no menu.",
    toolsBack: "Voltar às ferramentas",
    notFoundTitle: "Não encontramos essa página",
    notFoundBody:
      "Não encontramos um negócio com essa URL personalizada. Ele pode ter sido removido ou a URL pode estar incorreta.",
    tableNotFoundTitle: "Não encontramos essa mesa",
    tableNotFoundBody:
      "Esse código de mesa não está ativo. O QR pode ser antigo ou a mesa foi reiniciada. Peça à equipe um código atual.",
    goHome: "Ir para a página inicial",
    scanLabel: "Erro de leitura",
    scanTitle: "Não foi possível abrir o scanner.",
    scanBody:
      "Tente novamente em um momento, ou peça o código da mesa ao garçom.",
    scanAgain: "Escanear novamente",
    reservationBody: "Desculpe — sua reserva não foi afetada.",
    backToReservation: "Voltar à sua reserva",
    deliveryBody: "Desculpe — seu pedido de entrega não foi afetado.",
    backToTracking: "Voltar ao rastreamento do pedido",
  },
  it: {
    title: "Qualcosa è andato storto",
    body: "Ci scusiamo — il problema è nostro, non tuo.",
    tableBody:
      "Ci scusiamo — il problema è nostro, non tuo. Il tuo tavolo e ogni ordine effettuato sono al sicuro.",
    retry: "Riprova",
    back: "Indietro",
    home: "Home",
    notFound: "Non trovata",
    backToTable: "Torna al tuo tavolo",
    backToRestaurant: "Torna alla pagina del ristorante",
    errorCode: "Codice errore",
    profileHeading: "Impossibile caricare il profilo.",
    profileBody: "Riprova o accedi di nuovo.",
    profileBack: "Torna al profilo",
    toolsHeading: "Impossibile caricare lo strumento.",
    toolsBody: "Riprova, oppure scegli un altro strumento dal menu.",
    toolsBack: "Torna agli strumenti",
    notFoundTitle: "Non abbiamo trovato questa pagina",
    notFoundBody:
      "Non abbiamo trovato un'attività con questo URL personalizzato. Potrebbe essere stata rimossa o l'URL potrebbe essere errato.",
    tableNotFoundTitle: "Non abbiamo trovato questo tavolo",
    tableNotFoundBody:
      "Quel codice tavolo non è attivo. Il QR potrebbe essere vecchio o il tavolo è stato reimpostato. Chiedi al personale un codice aggiornato.",
    goHome: "Vai alla home",
    scanLabel: "Errore di scansione",
    scanTitle: "Non siamo riusciti ad aprire lo scanner.",
    scanBody:
      "Riprova tra un momento, oppure chiedi il codice del tavolo al cameriere.",
    scanAgain: "Scansiona di nuovo",
    reservationBody:
      "Ci scusiamo — la tua prenotazione non è stata interessata.",
    backToReservation: "Torna alla tua prenotazione",
    deliveryBody:
      "Ci scusiamo — il tuo ordine di consegna non è stato interessato.",
    backToTracking: "Torna al tracciamento dell'ordine",
  },
  zh: {
    title: "出了点问题",
    body: "抱歉——这是我们的问题，不是您的。",
    tableBody: "抱歉——这是我们的问题，不是您的。您的餐桌和已下订单都是安全的。",
    retry: "重试",
    back: "返回",
    home: "首页",
    notFound: "未找到",
    backToTable: "返回您的餐桌",
    backToRestaurant: "返回餐厅页面",
    errorCode: "错误代码",
    profileHeading: "无法加载您的个人资料。",
    profileBody: "请重试或重新登录。",
    profileBack: "返回个人资料",
    toolsHeading: "无法加载该工具。",
    toolsBody: "请重试，或从菜单中选择其他工具。",
    toolsBack: "返回工具",
    notFoundTitle: "我们找不到该页面",
    notFoundBody:
      "我们找不到使用该自定义网址的商家。它可能已被删除，或者网址拼写有误。",
    tableNotFoundTitle: "找不到该桌位",
    tableNotFoundBody:
      "该桌位码当前未激活。二维码可能已过期，或桌位已被重置。请向工作人员索取当前桌码。",
    goHome: "前往首页",
    scanLabel: "扫描错误",
    scanTitle: "我们无法打开扫描器。",
    scanBody: "请稍后再试，或向服务员索取桌号代码。",
    scanAgain: "重新扫描",
    reservationBody: "抱歉——您的预订未受影响。",
    backToReservation: "返回您的预订",
    deliveryBody: "抱歉——您的外卖订单未受影响。",
    backToTracking: "返回订单跟踪",
  },
  ja: {
    title: "問題が発生しました",
    body: "申し訳ありません — こちら側の問題です。",
    tableBody:
      "申し訳ありません — こちら側の問題です。テーブルとご注文は安全に保たれています。",
    retry: "再試行",
    back: "戻る",
    home: "ホーム",
    notFound: "見つかりません",
    backToTable: "テーブルに戻る",
    backToRestaurant: "レストランページに戻る",
    errorCode: "エラーコード",
    profileHeading: "プロフィールを読み込めませんでした。",
    profileBody: "もう一度お試しいただくか、再度サインインしてください。",
    profileBack: "プロフィールに戻る",
    toolsHeading: "ツールを読み込めませんでした。",
    toolsBody:
      "もう一度お試しいただくか、メニューから別のツールを選んでください。",
    toolsBack: "ツールに戻る",
    notFoundTitle: "そのページが見つかりませんでした",
    notFoundBody:
      "そのカスタムURLのビジネスが見つかりませんでした。削除されたか、URLが間違っている可能性があります。",
    tableNotFoundTitle: "テーブルが見つかりません",
    tableNotFoundBody:
      "そのテーブルコードは現在有効ではありません。QRコードが古いか、テーブルがリセットされた可能性があります。スタッフに新しいコードをお尋ねください。",
    goHome: "ホームへ移動",
    scanLabel: "スキャンエラー",
    scanTitle: "スキャナーを開けませんでした。",
    scanBody:
      "しばらくしてから再試行するか、スタッフにテーブルコードをお尋ねください。",
    scanAgain: "もう一度スキャン",
    reservationBody: "申し訳ありません — ご予約には影響ありません。",
    backToReservation: "予約に戻る",
    deliveryBody: "申し訳ありません — デリバリー注文には影響ありません。",
    backToTracking: "注文追跡に戻る",
  },
  ko: {
    title: "문제가 발생했습니다",
    body: "죄송합니다 — 저희 쪽 문제입니다.",
    tableBody:
      "죄송합니다 — 저희 쪽 문제입니다. 테이블과 주문하신 내용은 안전하게 보관됩니다.",
    retry: "다시 시도",
    back: "뒤로",
    home: "홈",
    notFound: "찾을 수 없음",
    backToTable: "테이블로 돌아가기",
    backToRestaurant: "레스토랑 페이지로 돌아가기",
    errorCode: "오류 코드",
    profileHeading: "프로필을 불러올 수 없습니다.",
    profileBody: "다시 시도하거나 다시 로그인해 주세요.",
    profileBack: "프로필로 돌아가기",
    toolsHeading: "도구를 불러올 수 없습니다.",
    toolsBody: "다시 시도하거나 메뉴에서 다른 도구를 선택해 주세요.",
    toolsBack: "도구로 돌아가기",
    notFoundTitle: "해당 페이지를 찾을 수 없습니다",
    notFoundBody:
      "해당 맞춤 URL의 비즈니스를 찾을 수 없습니다. 삭제되었거나 URL이 잘못 입력되었을 수 있습니다.",
    tableNotFoundTitle: "테이블을 찾을 수 없습니다",
    tableNotFoundBody:
      "해당 테이블 코드가 활성화되어 있지 않습니다. QR 코드가 오래되었거나 테이블이 초기화되었을 수 있습니다. 직원에게 현재 코드를 요청하세요.",
    goHome: "홈으로 이동",
    scanLabel: "스캔 오류",
    scanTitle: "스캐너를 열 수 없습니다.",
    scanBody: "잠시 후 다시 시도하거나 서버에게 테이블 코드를 요청하세요.",
    scanAgain: "다시 스캔",
    reservationBody: "죄송합니다 — 예약에는 영향이 없습니다.",
    backToReservation: "예약으로 돌아가기",
    deliveryBody: "죄송합니다 — 배달 주문에는 영향이 없습니다.",
    backToTracking: "주문 추적으로 돌아가기",
  },
  hi: {
    title: "कुछ गलत हो गया",
    body: "क्षमा करें — यह हमारी गलती है, आपकी नहीं।",
    tableBody:
      "क्षमा करें — यह हमारी गलती है, आपकी नहीं। आपकी टेबल और कोई भी ऑर्डर सुरक्षित हैं।",
    retry: "पुनः प्रयास करें",
    back: "वापस",
    home: "होम",
    notFound: "नहीं मिला",
    backToTable: "अपनी टेबल पर वापस जाएँ",
    backToRestaurant: "रेस्तरां पेज पर वापस जाएँ",
    errorCode: "त्रुटि कोड",
    profileHeading: "आपकी प्रोफ़ाइल लोड नहीं हो सकी।",
    profileBody: "फिर से कोशिश करें या फिर से साइन इन करें।",
    profileBack: "प्रोफ़ाइल पर वापस",
    toolsHeading: "टूल लोड नहीं हो सका।",
    toolsBody: "फिर से कोशिश करें, या मेनू से कोई और टूल चुनें।",
    toolsBack: "टूल्स पर वापस",
    notFoundTitle: "हमें वह पृष्ठ नहीं मिला",
    notFoundBody:
      "हमें उस कस्टम URL वाला कोई व्यवसाय नहीं मिला। हो सकता है इसे हटा दिया गया हो या URL गलत लिखा गया हो।",
    tableNotFoundTitle: "वह टेबल नहीं मिली",
    tableNotFoundBody:
      "यह टेबल कोड अभी सक्रिय नहीं है। QR कोड पुराना हो सकता है या टेबल रीसेट हुई हो। स्टाफ से वर्तमान कोड माँगें।",
    goHome: "होम पेज पर जाएं",
    scanLabel: "स्कैन त्रुटि",
    scanTitle: "हम स्कैनर नहीं खोल सके।",
    scanBody: "थोड़ी देर बाद फिर कोशिश करें, या अपने सर्वर से टेबल कोड माँगें।",
    scanAgain: "फिर से स्कैन करें",
    reservationBody: "क्षमा करें — आपकी आरक्षण प्रभावित नहीं हुई है।",
    backToReservation: "अपने आरक्षण पर वापस जाएँ",
    deliveryBody: "क्षमा करें — आपका डिलीवरी ऑर्डर प्रभावित नहीं हुआ है।",
    backToTracking: "ऑर्डर ट्रैकिंग पर वापस जाएँ",
  },
  nl: {
    title: "Er is iets misgegaan",
    body: "Sorry — dit ligt aan ons, niet aan jou.",
    tableBody:
      "Sorry — dit ligt aan ons, niet aan jou. Je tafel en eventuele bestelling zijn veilig.",
    retry: "Opnieuw proberen",
    back: "Terug",
    home: "Home",
    notFound: "Niet gevonden",
    backToTable: "Terug naar je tafel",
    backToRestaurant: "Terug naar de restaurantpagina",
    errorCode: "Foutcode",
    profileHeading: "Je profiel kon niet worden geladen.",
    profileBody: "Probeer het opnieuw of log opnieuw in.",
    profileBack: "Terug naar profiel",
    toolsHeading: "De tool kon niet worden geladen.",
    toolsBody: "Probeer het opnieuw, of kies een andere tool in het menu.",
    toolsBack: "Terug naar tools",
    notFoundTitle: "We konden die pagina niet vinden",
    notFoundBody:
      "We konden geen bedrijf met die aangepaste URL vinden. Het is mogelijk verwijderd of de URL is verkeerd gespeld.",
    tableNotFoundTitle: "We konden die tafel niet vinden",
    tableNotFoundBody:
      "Die tafelcode is niet actief. De QR-code is mogelijk oud of de tafel is gereset. Vraag het personeel om een actuele code.",
    goHome: "Naar de startpagina",
    scanLabel: "Scanfout",
    scanTitle: "We konden de scanner niet openen.",
    scanBody: "Probeer het zo opnieuw, of vraag je bediende om de tafelcode.",
    scanAgain: "Opnieuw scannen",
    reservationBody: "Sorry — je reservering is niet beïnvloed.",
    backToReservation: "Terug naar je reservering",
    deliveryBody: "Sorry — je bezorgorder is niet beïnvloed.",
    backToTracking: "Terug naar ordertracking",
  },
  no: {
    title: "Noe gikk galt",
    body: "Beklager — dette er på vår side, ikke din.",
    tableBody:
      "Beklager — dette er på vår side, ikke din. Bordet ditt og eventuelle bestillinger er trygge.",
    retry: "Prøv igjen",
    back: "Tilbake",
    home: "Hjem",
    notFound: "Ikke funnet",
    backToTable: "Tilbake til bordet ditt",
    backToRestaurant: "Tilbake til restaurantsiden",
    errorCode: "Feilkode",
    profileHeading: "Kunne ikke laste profilen din.",
    profileBody: "Prøv igjen eller logg inn på nytt.",
    profileBack: "Tilbake til profil",
    toolsHeading: "Kunne ikke laste verktøyet.",
    toolsBody: "Prøv igjen, eller velg et annet verktøy fra menyen.",
    toolsBack: "Tilbake til verktøy",
    notFoundTitle: "Vi fant ikke den siden",
    notFoundBody:
      "Vi fant ingen bedrift med den tilpassede URL-en. Den kan ha blitt fjernet, eller URL-en kan være feilstavet.",
    tableNotFoundTitle: "Vi fant ikke det bordet",
    tableNotFoundBody:
      "Den bordkoden er ikke aktiv. QR-koden kan være gammel, eller bordet er tilbakestilt. Be personalet om en gjeldende kode.",
    goHome: "Gå til forsiden",
    scanLabel: "Skannfeil",
    scanTitle: "Vi kunne ikke åpne skanneren.",
    scanBody:
      "Prøv igjen om et øyeblikk, eller spør serveren din om bordkoden.",
    scanAgain: "Skann på nytt",
    reservationBody: "Beklager — reservasjonen din er ikke påvirket.",
    backToReservation: "Tilbake til reservasjonen din",
    deliveryBody: "Beklager — leveringsordren din er ikke påvirket.",
    backToTracking: "Tilbake til ordresporing",
  },
  da: {
    title: "Noget gik galt",
    body: "Beklager — det er vores fejl, ikke din.",
    tableBody:
      "Beklager — det er vores fejl, ikke din. Dit bord og eventuelle ordrer er sikre.",
    retry: "Prøv igen",
    back: "Tilbage",
    home: "Hjem",
    notFound: "Ikke fundet",
    backToTable: "Tilbage til dit bord",
    backToRestaurant: "Tilbage til restaurantsiden",
    errorCode: "Fejlkode",
    profileHeading: "Kunne ikke indlæse din profil.",
    profileBody: "Prøv igen eller log ind igen.",
    profileBack: "Tilbage til profil",
    toolsHeading: "Kunne ikke indlæse værktøjet.",
    toolsBody: "Prøv igen, eller vælg et andet værktøj i menuen.",
    toolsBack: "Tilbage til værktøjer",
    notFoundTitle: "Vi kunne ikke finde den side",
    notFoundBody:
      "Vi kunne ikke finde en virksomhed med den brugerdefinerede URL. Den er måske blevet fjernet, eller URL'en er stavet forkert.",
    tableNotFoundTitle: "Vi kunne ikke finde det bord",
    tableNotFoundBody:
      "Den bordkode er ikke aktiv. QR-koden kan være gammel, eller bordet er blevet nulstillet. Spørg personalet om en aktuel kode.",
    goHome: "Gå til forsiden",
    scanLabel: "Scanningsfejl",
    scanTitle: "Vi kunne ikke åbne scanneren.",
    scanBody: "Prøv igen om et øjeblik, eller spørg din tjener om bordkoden.",
    scanAgain: "Scan igen",
    reservationBody: "Beklager — din reservation er upåvirket.",
    backToReservation: "Tilbage til din reservation",
    deliveryBody: "Beklager — din leveringsordre er upåvirket.",
    backToTracking: "Tilbage til ordresporing",
  },
  sv: {
    title: "Något gick fel",
    body: "Förlåt — det här är vårt fel, inte ditt.",
    tableBody:
      "Förlåt — det här är vårt fel, inte ditt. Ditt bord och eventuella beställningar är säkra.",
    retry: "Försök igen",
    back: "Tillbaka",
    home: "Hem",
    notFound: "Hittades inte",
    backToTable: "Tillbaka till ditt bord",
    backToRestaurant: "Tillbaka till restaurangsidan",
    errorCode: "Felkod",
    profileHeading: "Kunde inte ladda din profil.",
    profileBody: "Försök igen eller logga in på nytt.",
    profileBack: "Tillbaka till profil",
    toolsHeading: "Kunde inte ladda verktyget.",
    toolsBody: "Försök igen, eller välj ett annat verktyg från menyn.",
    toolsBack: "Tillbaka till verktyg",
    notFoundTitle: "Vi kunde inte hitta sidan",
    notFoundBody:
      "Vi kunde inte hitta ett företag med den anpassade URL:en. Det kan ha tagits bort eller så är URL:en felstavad.",
    tableNotFoundTitle: "Vi kunde inte hitta det bordet",
    tableNotFoundBody:
      "Den bordskoden är inte aktiv. QR-koden kan vara gammal eller så har bordet återställts. Be personalen om en aktuell kod.",
    goHome: "Gå till startsidan",
    scanLabel: "Skanningsfel",
    scanTitle: "Vi kunde inte öppna skannern.",
    scanBody: "Försök igen om en stund, eller be din servitör om bordskoden.",
    scanAgain: "Skanna igen",
    reservationBody: "Förlåt — din bokning påverkas inte.",
    backToReservation: "Tillbaka till din bokning",
    deliveryBody: "Förlåt — din leveransorder påverkas inte.",
    backToTracking: "Tillbaka till orderspårning",
  },
  pl: {
    title: "Coś poszło nie tak",
    body: "Przepraszamy — to nasza wina, nie Twoja.",
    tableBody:
      "Przepraszamy — to nasza wina, nie Twoja. Twój stolik i złożone zamówienia są bezpieczne.",
    retry: "Spróbuj ponownie",
    back: "Wstecz",
    home: "Strona główna",
    notFound: "Nie znaleziono",
    backToTable: "Wróć do swojego stolika",
    backToRestaurant: "Wróć do strony restauracji",
    errorCode: "Kod błędu",
    profileHeading: "Nie udało się wczytać profilu.",
    profileBody: "Spróbuj ponownie lub zaloguj się jeszcze raz.",
    profileBack: "Wróć do profilu",
    toolsHeading: "Nie udało się wczytać narzędzia.",
    toolsBody: "Spróbuj ponownie lub wybierz inne narzędzie z menu.",
    toolsBack: "Wróć do narzędzi",
    notFoundTitle: "Nie znaleźliśmy tej strony",
    notFoundBody:
      "Nie znaleźliśmy firmy o tym niestandardowym adresie URL. Mogła zostać usunięta lub adres URL jest błędnie wpisany.",
    tableNotFoundTitle: "Nie znaleźliśmy tego stolika",
    tableNotFoundBody:
      "Ten kod stolika nie jest aktywny. Kod QR może być stary albo stolik został zresetowany. Poproś obsługę o aktualny kod.",
    goHome: "Przejdź do strony głównej",
    scanLabel: "Błąd skanowania",
    scanTitle: "Nie udało się otworzyć skanera.",
    scanBody: "Spróbuj ponownie za chwilę lub poproś kelnera o kod stolika.",
    scanAgain: "Skanuj ponownie",
    reservationBody: "Przepraszamy — Twoja rezerwacja nie została naruszona.",
    backToReservation: "Wróć do rezerwacji",
    deliveryBody:
      "Przepraszamy — Twoje zamówienie dostawy nie zostało naruszone.",
    backToTracking: "Wróć do śledzenia zamówienia",
  },
  ru: {
    title: "Что-то пошло не так",
    body: "Извините — это с нашей стороны, не с вашей.",
    tableBody:
      "Извините — это с нашей стороны, не с вашей. Ваш стол и любые сделанные заказы в безопасности.",
    retry: "Повторить",
    back: "Назад",
    home: "Главная",
    notFound: "Не найдено",
    backToTable: "Вернуться к столу",
    backToRestaurant: "Вернуться на страницу ресторана",
    errorCode: "Код ошибки",
    profileHeading: "Не удалось загрузить профиль.",
    profileBody: "Попробуйте снова или войдите заново.",
    profileBack: "Вернуться к профилю",
    toolsHeading: "Не удалось загрузить инструмент.",
    toolsBody: "Попробуйте снова или выберите другой инструмент в меню.",
    toolsBack: "Вернуться к инструментам",
    notFoundTitle: "Мы не нашли эту страницу",
    notFoundBody:
      "Мы не нашли компанию с таким пользовательским URL. Возможно, она была удалена или URL указан с ошибкой.",
    tableNotFoundTitle: "Мы не нашли этот стол",
    tableNotFoundBody:
      "Этот код стола сейчас неактивен. QR-код может быть устаревшим, или стол сбросили. Попросите у персонала актуальный код.",
    goHome: "На главную",
    scanLabel: "Ошибка сканирования",
    scanTitle: "Не удалось открыть сканер.",
    scanBody:
      "Попробуйте снова через мгновение или попросите у официанта код стола.",
    scanAgain: "Сканировать снова",
    reservationBody: "Извините — ваша бронь не затронута.",
    backToReservation: "Вернуться к бронированию",
    deliveryBody: "Извините — ваш заказ на доставку не затронут.",
    backToTracking: "Вернуться к отслеживанию заказа",
  },
  tr: {
    title: "Bir şeyler ters gitti",
    body: "Üzgünüz — bu bizim tarafımızda, sizin değil.",
    tableBody:
      "Üzgünüz — bu bizim tarafımızda, sizin değil. Masanız ve verdiğiniz siparişler güvende.",
    retry: "Tekrar dene",
    back: "Geri",
    home: "Ana sayfa",
    notFound: "Bulunamadı",
    backToTable: "Masanıza dön",
    backToRestaurant: "Restoran sayfasına dön",
    errorCode: "Hata kodu",
    profileHeading: "Profiliniz yüklenemedi.",
    profileBody: "Tekrar deneyin veya yeniden oturum açın.",
    profileBack: "Profile dön",
    toolsHeading: "Araç yüklenemedi.",
    toolsBody: "Tekrar deneyin veya menüden başka bir araç seçin.",
    toolsBack: "Araçlara dön",
    notFoundTitle: "Bu sayfayı bulamadık",
    notFoundBody:
      "Bu özel URL'ye sahip bir işletme bulamadık. Kaldırılmış olabilir veya URL yanlış yazılmış olabilir.",
    tableNotFoundTitle: "O masayı bulamadık",
    tableNotFoundBody:
      "Bu masa kodu etkin değil. QR kod eski olabilir veya masa sıfırlanmış olabilir. Personelden güncel bir kod isteyin.",
    goHome: "Ana sayfaya git",
    scanLabel: "Tarama hatası",
    scanTitle: "Tarayıcıyı açamadık.",
    scanBody:
      "Biraz sonra tekrar deneyin veya garsonunuzdan masa kodunu isteyin.",
    scanAgain: "Tekrar tara",
    reservationBody: "Üzgünüz — rezervasyonunuz etkilenmedi.",
    backToReservation: "Rezervasyonunuza dön",
    deliveryBody: "Üzgünüz — teslimat siparişiniz etkilenmedi.",
    backToTracking: "Sipariş takibine dön",
  },
  th: {
    title: "เกิดข้อผิดพลาด",
    body: "ขออภัย — ปัญหานี้มาจากฝั่งเรา ไม่ใช่คุณ",
    tableBody:
      "ขออภัย — ปัญหานี้มาจากฝั่งเรา ไม่ใช่คุณ โต๊ะและคำสั่งซื้อของคุณยังปลอดภัย",
    retry: "ลองอีกครั้ง",
    back: "กลับ",
    home: "หน้าแรก",
    notFound: "ไม่พบ",
    backToTable: "กลับไปที่โต๊ะของคุณ",
    backToRestaurant: "กลับไปที่หน้าร้านอาหาร",
    errorCode: "รหัสข้อผิดพลาด",
    profileHeading: "โหลดโปรไฟล์ของคุณไม่ได้",
    profileBody: "ลองอีกครั้ง หรือลงชื่อเข้าใช้ใหม่",
    profileBack: "กลับไปที่โปรไฟล์",
    toolsHeading: "โหลดเครื่องมือไม่ได้",
    toolsBody: "ลองอีกครั้ง หรือเลือกเครื่องมืออื่นจากเมนู",
    toolsBack: "กลับไปที่เครื่องมือ",
    notFoundTitle: "เราไม่พบหน้านั้น",
    notFoundBody:
      "เราไม่พบธุรกิจที่ใช้ URL ที่กำหนดเองนั้น อาจถูกลบไปแล้วหรือ URL อาจสะกดผิด",
    tableNotFoundTitle: "ไม่พบโต๊ะนั้น",
    tableNotFoundBody:
      "รหัสโต๊ะนี้ยังไม่ใช้งาน รหัส QR อาจเก่าหรือโต๊ะถูกรีเซ็ต ขอรหัสปัจจุบันจากพนักงาน",
    goHome: "ไปที่หน้าแรก",
    scanLabel: "ข้อผิดพลาดในการสแกน",
    scanTitle: "เราไม่สามารถเปิดเครื่องสแกนได้",
    scanBody: "ลองอีกครั้งในอีกสักครู่ หรือขอรหัสโต๊ะจากพนักงานเสิร์ฟ",
    scanAgain: "สแกนอีกครั้ง",
    reservationBody: "ขออภัย — การจองของคุณไม่ได้รับผลกระทบ",
    backToReservation: "กลับไปที่การจองของคุณ",
    deliveryBody: "ขออภัย — คำสั่งจัดส่งของคุณไม่ได้รับผลกระทบ",
    backToTracking: "กลับไปที่การติดตามคำสั่ง",
  },
  vi: {
    title: "Đã xảy ra lỗi",
    body: "Xin lỗi — lỗi này từ phía chúng tôi, không phải bạn.",
    tableBody:
      "Xin lỗi — lỗi này từ phía chúng tôi, không phải bạn. Bàn và mọi đơn hàng của bạn vẫn an toàn.",
    retry: "Thử lại",
    back: "Quay lại",
    home: "Trang chủ",
    notFound: "Không tìm thấy",
    backToTable: "Quay lại bàn của bạn",
    backToRestaurant: "Quay lại trang nhà hàng",
    errorCode: "Mã lỗi",
    profileHeading: "Không thể tải hồ sơ của bạn.",
    profileBody: "Thử lại hoặc đăng nhập lại.",
    profileBack: "Quay lại hồ sơ",
    toolsHeading: "Không thể tải công cụ.",
    toolsBody: "Thử lại, hoặc chọn công cụ khác từ menu.",
    toolsBack: "Quay lại công cụ",
    notFoundTitle: "Chúng tôi không tìm thấy trang đó",
    notFoundBody:
      "Chúng tôi không tìm thấy doanh nghiệp với URL tùy chỉnh đó. Có thể nó đã bị xóa hoặc URL bị viết sai.",
    tableNotFoundTitle: "Chúng tôi không tìm thấy bàn đó",
    tableNotFoundBody:
      "Mã bàn đó hiện không hoạt động. Mã QR có thể đã cũ hoặc bàn đã được đặt lại. Hãy hỏi nhân viên mã hiện tại.",
    goHome: "Về trang chủ",
    scanLabel: "Lỗi quét",
    scanTitle: "Chúng tôi không thể mở trình quét.",
    scanBody: "Thử lại sau một chút, hoặc hỏi nhân viên mã bàn của bạn.",
    scanAgain: "Quét lại",
    reservationBody: "Xin lỗi — đặt bàn của bạn không bị ảnh hưởng.",
    backToReservation: "Quay lại đặt bàn của bạn",
    deliveryBody: "Xin lỗi — đơn giao hàng của bạn không bị ảnh hưởng.",
    backToTracking: "Quay lại theo dõi đơn hàng",
  },
};

/**
 * Resolve boundary copy for the active guest locale without using React context.
 * Call inside the component body (not at module scope) so language is read at render.
 */
export function resolveErrorBoundaryCopy(): BoundaryCopy {
  if (typeof window === "undefined") {
    return COPY.en;
  }

  const params = new URLSearchParams(window.location.search);
  const lang = (
    params.get("lang") ||
    document.documentElement.lang ||
    navigator.language ||
    "en"
  ).toLowerCase();

  // Argentine Spanish before generic Spanish (es-ar, es_ar, es-AR → es-AR).
  if (
    lang.startsWith("es-ar") ||
    lang === "es_ar" ||
    lang.startsWith("es_ar")
  ) {
    return COPY["es-AR"] ?? COPY.es ?? COPY.en;
  }
  if (lang.startsWith("es")) {
    return COPY.es ?? COPY.en;
  }

  const base = lang.split("-")[0] ?? "en";
  return COPY[base] ?? COPY.en;
}

/** Resolve provider-free boundary copy for server request handlers. */
export function resolveErrorBoundaryCopyForLocale(
  locale: StorefrontLocale,
): BoundaryCopy {
  return COPY[locale] ?? COPY.en;
}

/** Exported for unit tests — do not use in app code. */
export const __errorBoundaryCopyForTests = COPY;
