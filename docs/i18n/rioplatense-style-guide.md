# Rioplatense (Argentine) Spanish Style Guide — `es-AR`

This is the **single source of truth** for translating Payverge into Argentine
Spanish (`es-AR`). Every translator — human or agent — follows it so that
parallel work stays consistent. The generic `es` bundle stays **neutral /
Iberian Spanish**; `es-AR` is genuine **Rioplatense** (Buenos Aires / Río de
la Plata) Spanish.

> **Golden rule:** keep the JSON key shape and every ICU placeholder
> (`{name}`, `{count, plural, one {…} other {…}}`) **byte-identical** to `es`.
> Only the human-readable string values change. Never translate a `{var}`
> token; never switch `{var}` to `{{var}}` or vice versa.

---

## 1. Voseo — the defining feature

Argentine Spanish uses **vos**, not **tú**. This changes the second-person
singular in the **present indicative** and the **affirmative imperative**.
Everything else (usted, nosotros, plurals, subjunctive, negative imperative)
is unchanged from neutral Spanish.

### 1a. Present indicative (you-form statements/questions)
The stress moves to the last syllable and the stem does **not** diphthongize.

| Neutral (`es`, tú) | Argentine (`es-AR`, vos) |
|---|---|
| tienes | **tenés** |
| puedes | **podés** |
| quieres | **querés** |
| eres | **sos** |
| vas | vas *(unchanged)* |
| haces | **hacés** |
| pones | **ponés** |
| dices | **decís** |
| vienes | **venís** |
| necesitas | **necesitás** |
| pagas | **pagás** |
| eliges | **elegís** |
| pierdes | **perdés** |
| envías | enviás *(unchanged-looking; keep accent)* |
| recibes | **recibís** |
| confirmas | **confirmás** |
| guardas | **guardás** |

### 1b. Affirmative imperative (you-form commands — very common in UI buttons)
Drop the final `-r` of the infinitive and add an accent on the final vowel.
**No diphthong.**

| Neutral (`es`) | Argentine (`es-AR`) | Infinitive |
|---|---|---|
| Guarda | **Guardá** | guardar |
| Configura | **Configurá** | configurar |
| Elige | **Elegí** | elegir |
| Añade / Agrega | **Agregá** | agregar |
| Selecciona | **Seleccioná** | seleccionar |
| Ingresa | **Ingresá** | ingresar |
| Introduce | **Ingresá** | (use ingresar) |
| Rellena / Completa | **Completá** | completar |
| Pulsa / Haz clic | **Tocá** / **Hacé clic** | tocar / hacer |
| Escribe | **Escribí** | escribir |
| Revisa | **Revisá** | revisar |
| Confirma | **Confirmá** | confirmar |
| Comparte | **Compartí** | compartir |
| Empieza / Comienza | **Empezá** / **Comenzá** | empezar |
| Vuelve | **Volvé** | volver |
| Elige una opción | **Elegí una opción** | elegir |
| Ten en cuenta | **Tené en cuenta** | tener |
| Ve a… | **Andá a…** | ir |
| Haz | **Hacé** | hacer |
| Pon | **Poné** | poner |
| Ven | **Vení** | venir |
| Sé | **Sé** *(unchanged)* | ser |

### 1c. What does NOT change
- **usted** forms (formal): identical. We use **vos (informal)** as the default
  register throughout the app — friendly, modern, the norm in Argentine
  consumer products. Do **not** use usted unless a string is explicitly formal.
- Negative imperative: "No olvides" → **"No te olvides"** (uses subjunctive,
  same form) — only the reflexive/pronoun differs, the verb stays "olvides".
- Plural "you" (ustedes), nosotros, third person, infinitives, gerunds.
- Possessives/pronouns: "tu", "tus", "te", "ti" → still **tu/tus/te** with vos
  (vos uses "tu/tus" for possessive and "te" as object pronoun). Note **"ti"
  → "vos"** ("para ti" → "para vos").

---

## 2. Glossary (es → es-AR)

Replace these whenever they appear. Left = neutral Spanish that may appear in
the `es` bundle; right = Argentine.

### Restaurant / dining domain (highest priority — these make diners feel at home)
| Neutral | Argentine | Notes |
|---|---|---|
| camarero / mesero | **mozo** | the waiter |
| factura (= open table bill) | **cuenta** | In AR "factura" = the AFIP fiscal invoice (and also a pastry!). The open table bill/tab is **la cuenta**. In the bill-management UI (billManager, billCreator, billDetailsModal) translate the *open-bill* sense of "factura" → "cuenta". Keep "factura" only where it genuinely means the fiscal/tax invoice (fiscal.json). |
| la cuenta | **la cuenta** | ✓ same |
| propina | **propina** | ✓ same |
| menú / carta | **menú** | ✓ same (both fine; prefer "menú") |
| pedido | **pedido** | ✓ same |
| reserva | **reserva** | ✓ same |
| mesa | **mesa** | ✓ same |
| zumo | **jugo** | juice |
| patatas | **papas** | potatoes/fries |
| bocadillo | **sándwich** | |
| tarta | **torta** | cake |
| refresco | **gaseosa** | soft drink |
| aperitivo | **picada / entrada** | |

### App / fintech / UI domain
| Neutral | Argentine | Notes |
|---|---|---|
| móvil | **celular** | mobile phone |
| ordenador | **computadora** | computer |
| coche | **auto** | car (delivery) |
| aparcar | **estacionar** | |
| vale / de acuerdo | **dale / listo** | "OK"/"got it" (casual) |
| pulsar / hacer clic | **tocar / hacer clic** | tap on mobile = "tocá" |
| introducir (datos) | **ingresar** | enter data |
| rellenar | **completar** | fill in |
| fichero | **archivo** | file |
| enlace | **link / enlace** | both fine; "link" is common |
| actualizar | **actualizar** | ✓ same |
| ahora mismo | **ahora / en este momento** | |
| coger | **agarrar / tomar** | never "coger" (vulgar in AR) |
| ordenar (un pedido) | **hacer / realizar (un pedido)** | |
| email / correo | **email / correo** | both fine |
| usuario / contraseña | **usuario / contraseña** | ✓ same |

### Money / numbers
- Currency symbol & formatting are handled by code (`localeUnits`/currency),
  **not** by translation strings — do not hardcode `$`/`ARS` into translations.
- Decimal separator in prose: Argentine uses comma for decimals, point for
  thousands — but **do not** reformat interpolated `{amount}` values; the code
  formats them. Only adjust separators in static example text.
- Example placeholders: phone `+54 9 11 1234-5678`; email `juan@ejemplo.com.ar`;
  address `Av. Corrientes 1234`, city `Buenos Aires`, province `CABA`,
  country `Argentina`; postal code `C1043`.

---

## 3. Tone & register
- **Warm, direct, modern, professional.** Argentine consumer/fintech products
  (Mercado Pago, Ualá, Naranja X) speak informally with **vos** but stay
  polished. Match that.
- Prefer active voice and short verbs. "Configurá tu negocio" beats
  "Realice la configuración de su negocio".
- Keep brand/product nouns (Payverge, USDC, QR, Stripe, PayPal) untranslated.
- Don't over-localize technical/web3 terms diners won't know — keep them as in
  `es` unless the glossary says otherwise.

---

## 4. Quality checklist (run before committing any es-AR file)
- [ ] Every `{placeholder}` and ICU block matches `es` exactly (count + names).
- [ ] No Peninsular `tú`-form leaks: search values for `tienes`, `puedes`,
      `quieres`, `debes`, `eres`, `configura `, `elige `, `pulsa`,
      `rellena`, `introduce`, `añade`, `selecciona ` (imperative), `haz clic`.
      Each should be voseo (tenés/podés/querés/debés/sos/configurá/elegí/
      tocá/completá/ingresá/agregá/seleccioná/hacé clic).
- [ ] No glossary misses: `camarero`, `móvil`, `ordenador`, `zumo`, `coche`,
      `aparcar`, `coger`, `vale `.
- [ ] Reads naturally to a porteño — say it out loud.
- [ ] JSON is valid; key set unchanged vs the es base.

---

## 5. For the override architecture (operator tier)
es-AR operator files are **override layers** over the `es` base — they contain
**only the keys whose value actually differs** from `es` (voseo verbs, glossary
swaps). Keys identical to `es` are **omitted** (they inherit via deepMerge).
This keeps es-AR DRY and means a string that needs no Argentine change is never
duplicated. The **guest** bundle (`guest-messages/es-AR.json`) is the
exception: it is a single self-complete file (the guest provider loads one JSON
per locale), so it must contain **every** key.
