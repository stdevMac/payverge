package langid

import "testing"

func TestDetectLatin(t *testing.T) {
	cases := []struct {
		lang string
		text string
	}{
		{"en", "Welcome to our restaurant, how can I help you with your order today?"},
		{"es", "Bienvenido a nuestro restaurante, ¿en qué puedo ayudarte con tu pedido?"},
		{"fr", "Bienvenue dans notre restaurant, comment puis-je vous aider avec votre commande?"},
		{"de", "Willkommen in unserem Restaurant, wie kann ich Ihnen mit Ihrer Bestellung helfen?"},
		{"it", "Benvenuto nel nostro ristorante, come posso aiutarti con il tuo ordine oggi?"},
		{"pt", "Bem-vindo ao nosso restaurante, como posso ajudar você com o seu pedido hoje?"},
		{"nl", "Welkom in ons restaurant, hoe kan ik u helpen met uw bestelling vandaag?"},
		{"tr", "Restoranımıza hoş geldiniz, siparişinizle ilgili size nasıl yardımcı olabilirim?"},
	}
	for _, tc := range cases {
		got, conf := Detect(tc.text)
		if got != tc.lang {
			t.Errorf("Detect(%q) = %q (conf %.2f), want %q", tc.text, got, conf, tc.lang)
		}
	}
}

func TestDetectScript(t *testing.T) {
	cases := []struct {
		lang string
		text string
	}{
		{"ar", "مرحبا بكم في مطعمنا، كيف يمكنني مساعدتك في طلبك اليوم؟"},
		{"ru", "Добро пожаловать в наш ресторан, чем я могу помочь с вашим заказом?"},
		{"ja", "私たちのレストランへようこそ、ご注文のお手伝いをいたします。"},
		{"ko", "저희 레스토랑에 오신 것을 환영합니다, 주문을 도와드리겠습니다."},
		{"zh", "欢迎光临本餐厅，今天我能帮您点餐吗？"},
		{"hi", "हमारे रेस्तरां में आपका स्वागत है, मैं आपके ऑर्डर में कैसे मदद कर सकता हूँ?"},
		{"th", "ยินดีต้อนรับสู่ร้านอาหารของเรา ฉันจะช่วยคุณสั่งอาหารได้อย่างไร"},
	}
	for _, tc := range cases {
		got, conf := Detect(tc.text)
		if got != tc.lang {
			t.Errorf("Detect(%q) = %q (conf %.2f), want %q", tc.text, got, conf, tc.lang)
		}
	}
}

func TestDetectEmptyAndShort(t *testing.T) {
	if got, conf := Detect(""); got != "" || conf != 0 {
		t.Fatalf(`Detect("") = %q/%.2f, want ""/0`, got, conf)
	}
	// A single ASCII token has no stopword signal -> low confidence, not a crash.
	got, conf := Detect("ok")
	if conf > 0.5 {
		t.Fatalf("Detect(%q) too confident: %q/%.2f", "ok", got, conf)
	}
}
