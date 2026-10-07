package services

var waiterOffTopicRedirects = map[string]string{
	"en":    "I'm here to help with our menu and your visit. What can I get you?",
	"es":    "Estoy aquí para ayudarte con nuestro menú y tu visita. ¿Qué te puedo ofrecer?",
	"es-AR": "Estoy acá para ayudarte con el menú y tu visita. ¿Qué te puedo ofrecer?",
	"fr":    "Je suis là pour vous aider avec notre menu et votre visite. Que puis-je vous proposer ?",
	"de":    "Ich bin hier, um Ihnen mit unserer Speisekarte und Ihrem Besuch zu helfen. Was kann ich Ihnen anbieten?",
	"it":    "Sono qui per aiutarti con il nostro menù e la tua visita. Cosa posso offrirti?",
	"pt":    "Estou aqui para ajudar com o nosso cardápio e a sua visita. O que posso lhe oferecer?",
	"nl":    "Ik ben hier om te helpen met ons menu en uw bezoek. Wat kan ik voor u doen?",
	"da":    "Jeg er her for at hjælpe med vores menu og dit besøg. Hvad kan jeg tilbyde dig?",
	"no":    "Jeg er her for å hjelpe med menyen og besøket ditt. Hva kan jeg tilby deg?",
	"sv":    "Jag är här för att hjälpa till med vår meny och ditt besök. Vad kan jag erbjuda dig?",
	"pl":    "Jestem tutaj, aby pomóc z naszym menu i Twoją wizytą. Co mogę Ci zaproponować?",
	"ru":    "Я здесь, чтобы помочь с нашим меню и вашим визитом. Что я могу вам предложить?",
	"tr":    "Menümüz ve ziyaretinizle ilgili yardımcı olmak için buradayım. Size ne önerebilirim?",
	"ar":    "أنا هنا للمساعدة في قائمتنا وزيارتك. ماذا يمكنني أن أقدم لك؟",
	"hi":    "मैं हमारे मेनू और आपकी यात्रा में मदद करने के लिए यहाँ हूँ। मैं आपको क्या दे सकता हूँ?",
	"ja":    "メニューやご来店についてのお手伝いをいたします。何をお持ちしましょうか？",
	"ko":    "메뉴와 방문에 도움을 드리기 위해 여기 있습니다. 무엇을 도와드릴까요?",
	"th":    "ฉันอยู่ที่นี่เพื่อช่วยเกี่ยวกับเมนูและการเยี่ยมชมของคุณ มีอะไรให้ฉันช่วยไหม?",
	"vi":    "Tôi ở đây để giúp bạn với thực đơn và chuyến thăm của bạn. Tôi có thể giúp gì cho bạn?",
	"zh":    "我在这里帮您了解菜单和您的用餐体验。您想要点什么呢？",
}

var waiterAbuseDeclines = map[string]string{
	"en":    "Let's keep things respectful. I'm happy to help with the menu whenever you're ready.",
	"es":    "Mantengamos el respeto. Estaré encantado de ayudarte con el menú cuando quieras.",
	"es-AR": "Mantengamos el respeto. Estaré encantado de ayudarte con el menú cuando quieras.",
	"fr":    "Restons respectueux. Je serai ravi de vous aider avec le menu quand vous serez prêt.",
	"de":    "Lassen Sie uns respektvoll bleiben. Ich helfe Ihnen gerne mit der Speisekarte, wenn Sie bereit sind.",
	"it":    "Manteniamo il rispetto. Sarò felice di aiutarti con il menù quando sarai pronto.",
	"pt":    "Vamos manter o respeito. Ficarei feliz em ajudar com o cardápio quando estiver pronto.",
	"nl":    "Laten we het respectvol houden. Ik help u graag met het menu wanneer u klaar bent.",
	"da":    "Lad os holde det respektfuldt. Jeg hjælper gerne med menuen, når du er klar.",
	"no":    "La oss holde det respektfullt. Jeg hjelper gjerne med menyen når du er klar.",
	"sv":    "Låt oss hålla det respektfullt. Jag hjälper gärna till med menyn när du är redo.",
	"pl":    "Zachowajmy szacunek. Chętnie pomogę z menu, gdy będziesz gotowy.",
	"ru":    "Давайте сохранять уважение. Я буду рад помочь с меню, когда вы будете готовы.",
	"tr":    "Saygılı olalım. Hazır olduğunuzda menü konusunda yardımcı olmaktan mutluluk duyarım.",
	"ar":    "لنحافظ على الاحترام. سأكون سعيداً بمساعدتك في القائمة عندما تكون مستعداً.",
	"hi":    "आइए सम्मान बनाए रखें। जब आप तैयार हों तो मुझे मेनू में मदद करने में खुशी होगी।",
	"ja":    "敬意を持って接しましょう。準備ができたら、メニューのお手伝いをいたします。",
	"ko":    "서로 존중합시다. 준비되시면 메뉴 도와드리겠습니다.",
	"th":    "มารักษาความเคารพซึ่งกันและกัน ฉันยินดีช่วยเหลือเกี่ยวกับเมนูเมื่อคุณพร้อม",
	"vi":    "Hãy giữ sự tôn trọng. Tôi sẵn lòng giúp bạn với thực đơn khi bạn sẵn sàng.",
	"zh":    "让我们保持尊重。当您准备好时，我很乐意帮您了解菜单。",
}

var waiterHumanAssisting = map[string]string{
	"en":    "Please hold on a moment — one of our team members is assisting you directly and will reply here shortly.",
	"es":    "Espera un momento, por favor: uno de los miembros de nuestro equipo te está atendiendo directamente y te responderá aquí en breve.",
	"es-AR": "Aguardá un momento, por favor: alguien de nuestro equipo te está atendiendo directamente y te va a responder acá enseguida.",
	"fr":    "Un instant s'il vous plaît — un membre de notre équipe s'occupe de vous directement et vous répondra ici sous peu.",
	"de":    "Bitte einen Moment Geduld — jemand aus unserem Team kümmert sich direkt um Sie und antwortet gleich hier.",
	"it":    "Attendi un momento, per favore: un membro del nostro team ti sta assistendo direttamente e ti risponderà qui a breve.",
	"pt":    "Aguarde um momento, por favor: um membro da nossa equipe está lhe atendendo diretamente e responderá aqui em breve.",
	"nl":    "Een ogenblik geduld alstublieft — een van onze medewerkers helpt u rechtstreeks en reageert hier zo.",
	"da":    "Vent venligst et øjeblik — en af vores medarbejdere hjælper dig direkte og svarer her om lidt.",
	"no":    "Vent et øyeblikk — en av våre medarbeidere hjelper deg direkte og svarer her snart.",
	"sv":    "Vänta ett ögonblick — en av våra medarbetare hjälper dig direkt och svarar här strax.",
	"pl":    "Chwileczkę — jeden z członków naszego zespołu obsługuje Cię bezpośrednio i wkrótce odpowie tutaj.",
	"ru":    "Пожалуйста, подождите немного — один из наших сотрудников уже помогает вам и вскоре ответит здесь.",
	"tr":    "Lütfen bir dakika bekleyin — ekibimizden biri size doğrudan yardımcı oluyor ve birazdan buradan yanıtlayacak.",
	"ar":    "يرجى الانتظار لحظة — أحد أعضاء فريقنا يساعدك مباشرة وسيرد هنا قريباً.",
	"hi":    "कृपया एक क्षण प्रतीक्षा करें — हमारी टीम का एक सदस्य आपकी सीधे सहायता कर रहा है और शीघ्र ही यहाँ उत्तर देगा।",
	"ja":    "少々お待ちください。スタッフが直接対応しており、まもなくこちらでお返事いたします。",
	"ko":    "잠시만 기다려 주세요 — 저희 팀원이 직접 도와드리고 있으며 곧 여기에서 답변드리겠습니다.",
	"th":    "กรุณารอสักครู่ — ทีมงานของเรากำลังดูแลคุณโดยตรงและจะตอบกลับที่นี่ในไม่ช้า",
	"vi":    "Vui lòng chờ một chút — một thành viên trong nhóm của chúng tôi đang trực tiếp hỗ trợ bạn và sẽ trả lời tại đây ngay.",
	"zh":    "请稍候，我们的团队成员正在直接为您服务，稍后会在这里回复您。",
}

// WaiterOffTopicRedirect returns the localized off-topic redirect message.
func WaiterOffTopicRedirect(code string) string { return allergenCopy(waiterOffTopicRedirects, code) }

// WaiterHumanAssisting returns the localized "a human is assisting you" ack shown
// to guests while a staff member has taken over (paused) the conversation.
func WaiterHumanAssisting(code string) string { return allergenCopy(waiterHumanAssisting, code) }

// WaiterAbuseDecline returns the localized abuse decline message.
func WaiterAbuseDecline(code string) string { return allergenCopy(waiterAbuseDeclines, code) }
