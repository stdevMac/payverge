package ops_guides

// Stable guide shells — locale copy is overlaid in enGuides/esGuides/esARGuides.
func baseGuides() []Guide {
	return []Guide{
		{ID: "overview-get-started", Tab: "overview", Destination: "tab:overview", RequiredPermission: "overview:read", FollowUpIDs: []string{"menu-add-item", "tables-create-qr"}},
		{ID: "bills-open-close", Tab: "bills", Destination: "tab:bills", RequiredPermission: "bills:read", FollowUpIDs: []string{"cash-register-shift"}},
		{ID: "cash-register-shift", Tab: "cash-register", Destination: "tab:cash-register", RequiredPermission: "cash_register:read", FollowUpIDs: []string{"bills-open-close"}},
		{ID: "printers-connect", Tab: "printers", Destination: "route:settings/printers", RequiredPermission: "printers:read", FollowUpIDs: []string{"bills-open-close"}},
		{ID: "kitchen-order-flow", Tab: "kitchen", Destination: "tab:kitchen", RequiredPermission: "kitchen:read", FollowUpIDs: []string{"menu-add-item"}},
		{ID: "reservations-manage", Tab: "reservations", Destination: "tab:reservations", RequiredPermission: "reservations:read", FollowUpIDs: []string{"tables-create-qr"}},
		{ID: "menu-add-item", Tab: "menu", Destination: "tab:menu", RequiredPermission: "menu:write", WorkflowID: "first_menu_item", FollowUpIDs: []string{"tables-create-qr", "ai-waiter-configure"}},
		{ID: "tables-create-qr", Tab: "tables", Destination: "tab:tables", RequiredPermission: "tables:write", WorkflowID: "first_table_qr", FollowUpIDs: []string{"menu-add-item"}},
		{ID: "guest-order-experience", Tab: "tables", Destination: "tab:tables", RequiredPermission: "tables:read", FollowUpIDs: []string{"tables-create-qr", "ai-waiter-configure"}},
		{ID: "ai-waiter-configure", Tab: "ai-waiter", Destination: "tab:ai-waiter", RequiredPermission: "ai_waiter:read", FollowUpIDs: []string{"director-boundary"}},
		// #874: pausing the AI or answering a guest yourself is a shipped path
		// (Live Monitor takeover, POST .../ai/conversations/:convId/pause, gated on
		// ai_waiter:reply). Without a guide for it, "pause the AI" fell into the
		// setup guide above.
		{ID: "ai-waiter-takeover", Tab: "ai-waiter", Destination: "tab:ai-waiter", RequiredPermission: "ai_waiter:reply", FollowUpIDs: []string{"ai-waiter-configure"}},
		{ID: "director-boundary", Tab: "director-console", Destination: "tab:director-console", RequiredPermission: "director:read", FollowUpIDs: []string{"analytics-read"}},
		{ID: "marketing-create-draft", Tab: "marketing", Destination: "tab:marketing", RequiredPermission: "marketing:write", FollowUpIDs: []string{"menu-add-item"}},
		{ID: "analytics-read", Tab: "analytics", Destination: "tab:analytics", RequiredPermission: "analytics:read", FollowUpIDs: []string{"director-boundary"}},
		{ID: "crm-customer", Tab: "crm", Destination: "tab:crm", RequiredPermission: "crm:read", FollowUpIDs: []string{"marketing-create-draft"}},
		{ID: "delivery-configure", Tab: "delivery", Destination: "tab:delivery", RequiredPermission: "delivery:write", FollowUpIDs: []string{"menu-add-item"}},
		{ID: "counter-use", Tab: "counter", Destination: "tab:counter", RequiredPermission: "counter:read", FollowUpIDs: []string{"bills-open-close"}},
		{ID: "inventory-stock", Tab: "inventory", Destination: "tab:inventory", RequiredPermission: "inventory:read", FollowUpIDs: []string{"menu-add-item"}},
		{ID: "staff-invite", Tab: "staff", Destination: "tab:staff", RequiredPermission: "staff:invite", FollowUpIDs: []string{"schedule-build"}},
		{ID: "schedule-build", Tab: "schedule", Destination: "tab:schedule", RequiredPermission: "schedule:write", FollowUpIDs: []string{"staff-invite"}},
		{ID: "business-page-publish", Tab: "business-page", Destination: "tab:business-page", RequiredPermission: "business_page:write", FollowUpIDs: []string{"settings-update"}},
		{ID: "accounting-review", Tab: "accounting", Destination: "tab:accounting", RequiredPermission: "accounting:read", FollowUpIDs: []string{"fiscal-setup"}},
		{ID: "fiscal-setup", Tab: "fiscal", Destination: "tab:fiscal", RequiredPermission: "fiscal:write", FollowUpIDs: []string{"accounting-review"}},
		{ID: "plugins-connect", Tab: "plugins", Destination: "tab:plugins", RequiredPermission: "plugins:write", FollowUpIDs: []string{"settings-update"}},
		{ID: "settings-update", Tab: "settings", Destination: "tab:settings", RequiredPermission: "settings:write", FollowUpIDs: []string{"business-page-publish"}},
		{ID: "support-contact", Tab: "overview", Destination: "tab:overview", RequiredPermission: "overview:read", FollowUpIDs: []string{"overview-get-started"}},
	}
}

func withCopy(base Guide, phrases []string, answer string, steps []string) Guide {
	base.Phrases = phrases
	base.Answer = answer
	base.Steps = steps
	return base
}

type guidePresentation struct {
	followUpLabel    string
	followUpPrompt   string
	destinationLabel string
}

func presentation(label, prompt, destination string) guidePresentation {
	return guidePresentation{followUpLabel: label, followUpPrompt: prompt, destinationLabel: destination}
}

func applyGuidePresentation(locale string, guides []Guide) []Guide {
	presentations := englishGuidePresentations()
	if locale == "es" || locale == "es-AR" {
		presentations = spanishGuidePresentations()
	}
	for i := range guides {
		copy, ok := presentations[guides[i].ID]
		if !ok {
			continue
		}
		guides[i].FollowUpLabel = copy.followUpLabel
		guides[i].FollowUpPrompt = copy.followUpPrompt
		guides[i].DestinationLabel = copy.destinationLabel
	}
	if locale == "es-AR" {
		for i := range guides {
			switch guides[i].ID {
			case "ai-waiter-configure":
				guides[i].FollowUpLabel = "Configurar Mozo IA"
				guides[i].FollowUpPrompt = "¿Cómo configuro Mozo IA?"
				guides[i].DestinationLabel = "Abrir Mozo IA"
			case "ai-waiter-takeover":
				guides[i].FollowUpLabel = "Pausar la IA o intervenir un chat"
				guides[i].FollowUpPrompt = "¿Cómo pauso Mozo IA o intervengo un chat?"
				guides[i].DestinationLabel = "Abrir Mozo IA"
			}
		}
	}
	return guides
}

func englishGuidePresentations() map[string]guidePresentation {
	return map[string]guidePresentation{
		"overview-get-started":   presentation("Complete setup", "How do I finish the initial setup?", "Open Overview"),
		"bills-open-close":       presentation("Manage an open bill", "How do I review and close an open bill?", "Open Bills"),
		"cash-register-shift":    presentation("Manage a cash shift", "How do I open or close a cash-register shift?", "Open Cash Register"),
		"printers-connect":       presentation("Connect a printer", "How do I connect and test a printer?", "Open Printers"),
		"kitchen-order-flow":     presentation("Manage kitchen tickets", "How does the kitchen order flow work?", "Open Kitchen"),
		"reservations-manage":    presentation("Manage reservations", "How do I manage reservations and the waitlist?", "Open Reservations"),
		"menu-add-item":          presentation("Add a menu item", "How do I add a menu item?", "Open Menu"),
		"tables-create-qr":       presentation("Create a table QR code", "How do I create a table and download its QR code?", "Open Tables"),
		"guest-order-experience": presentation("See how guests order", "How do guests order from the diner menu?", "Open Tables"),
		"ai-waiter-configure":    presentation("Configure AI Waiter", "How do I configure AI Waiter?", "Open AI Waiter"),
		"ai-waiter-takeover":     presentation("Pause AI or take over a chat", "How do I pause AI Waiter or take over a guest chat?", "Open AI Waiter"),
		"director-boundary":      presentation("Understand Director Console", "What can Director Console do that Ops cannot?", "Open Director Console"),
		"marketing-create-draft": presentation("Create a marketing draft", "How do I create and review a marketing draft?", "Open Marketing"),
		"analytics-read":         presentation("Review analytics", "How do I read sales and traffic analytics?", "Open Analytics"),
		"crm-customer":           presentation("Review a customer profile", "How do I find a customer and manage profile notes?", "Open CRM"),
		"delivery-configure":     presentation("Configure delivery", "How do I configure delivery zones and fees?", "Open Delivery"),
		"counter-use":            presentation("Create a counter order", "How do I create a counter order?", "Open Counter"),
		"inventory-stock":        presentation("Review inventory", "How do I review stock and unavailable items?", "Open Inventory"),
		"staff-invite":           presentation("Invite a staff member", "How do I invite staff and assign a role?", "Open Staff"),
		"schedule-build":         presentation("Build a staff schedule", "How do I build a weekly staff schedule?", "Open Schedule"),
		"business-page-publish":  presentation("Publish the business page", "How do I review and publish the business page?", "Open Business Page"),
		"accounting-review":      presentation("Review accounting", "How do I review accounting activity?", "Open Accounting"),
		"fiscal-setup":           presentation("Configure fiscal receipts", "How do I configure official fiscal receipts?", "Open Fiscal"),
		"plugins-connect":        presentation("Connect a plugin", "How do I connect and test a plugin?", "Open Plugins"),
		"settings-update":        presentation("Update business settings", "How do I update business settings?", "Open Settings"),
		"support-contact":        presentation("Contact Payverge support", "How do I contact Payverge support?", "Open Overview"),
	}
}

func spanishGuidePresentations() map[string]guidePresentation {
	return map[string]guidePresentation{
		"overview-get-started":   presentation("Completar la configuración", "¿Cómo completo la configuración inicial?", "Abrir Resumen"),
		"bills-open-close":       presentation("Gestionar una cuenta abierta", "¿Cómo reviso y cierro una cuenta abierta?", "Abrir Cuentas"),
		"cash-register-shift":    presentation("Gestionar un turno de caja", "¿Cómo abro o cierro un turno de caja?", "Abrir Caja"),
		"printers-connect":       presentation("Conectar una impresora", "¿Cómo conecto y pruebo una impresora?", "Abrir Impresoras"),
		"kitchen-order-flow":     presentation("Gestionar comandas de cocina", "¿Cómo funciona el flujo de pedidos en cocina?", "Abrir Cocina"),
		"reservations-manage":    presentation("Gestionar reservaciones", "¿Cómo gestiono las reservaciones y la lista de espera?", "Abrir Reservaciones"),
		"menu-add-item":          presentation("Agregar un producto al menú", "¿Cómo agrego un producto al menú?", "Abrir Menú"),
		"tables-create-qr":       presentation("Crear un código QR de mesa", "¿Cómo creo una mesa y descargo su código QR?", "Abrir Mesas"),
		"guest-order-experience": presentation("Ver cómo piden los comensales", "¿Cómo piden los comensales desde el menú?", "Abrir Mesas"),
		"ai-waiter-configure":    presentation("Configurar Camarero IA", "¿Cómo configuro Camarero IA?", "Abrir Camarero IA"),
		"ai-waiter-takeover":     presentation("Pausar la IA o intervenir un chat", "¿Cómo pauso Camarero IA o intervengo un chat?", "Abrir Camarero IA"),
		"director-boundary":      presentation("Entender la Consola del Director", "¿Qué puede hacer la Consola del Director que Ops no puede?", "Abrir Consola del Director"),
		"marketing-create-draft": presentation("Crear un borrador de marketing", "¿Cómo creo y reviso un borrador de marketing?", "Abrir Marketing"),
		"analytics-read":         presentation("Revisar analíticas", "¿Cómo interpreto las analíticas de ventas y tráfico?", "Abrir Analíticas"),
		"crm-customer":           presentation("Revisar un perfil de cliente", "¿Cómo encuentro un cliente y gestiono sus notas?", "Abrir CRM"),
		"delivery-configure":     presentation("Configurar entregas", "¿Cómo configuro las zonas y los cargos de entrega?", "Abrir Entrega"),
		"counter-use":            presentation("Crear un pedido de mostrador", "¿Cómo creo un pedido de mostrador?", "Abrir Mostradores"),
		"inventory-stock":        presentation("Revisar el inventario", "¿Cómo reviso las existencias y los productos agotados?", "Abrir Inventario"),
		"staff-invite":           presentation("Invitar a una persona", "¿Cómo invito al personal y asigno un rol?", "Abrir Personal"),
		"schedule-build":         presentation("Organizar el horario", "¿Cómo organizo el horario semanal del personal?", "Abrir Horario"),
		"business-page-publish":  presentation("Publicar la página del negocio", "¿Cómo reviso y publico la página del negocio?", "Abrir Página de Negocio"),
		"accounting-review":      presentation("Revisar la contabilidad", "¿Cómo reviso la actividad contable?", "Abrir Contabilidad"),
		"fiscal-setup":           presentation("Configurar comprobantes fiscales", "¿Cómo configuro la emisión de comprobantes fiscales?", "Abrir Facturas"),
		"plugins-connect":        presentation("Conectar un plugin", "¿Cómo conecto y pruebo un plugin?", "Abrir Plugins"),
		"settings-update":        presentation("Actualizar la configuración", "¿Cómo actualizo la configuración del negocio?", "Abrir Configuración"),
		"support-contact":        presentation("Contactar con soporte", "¿Cómo contacto con el soporte de Payverge?", "Abrir Resumen"),
	}
}

func enGuides() []Guide {
	b := baseGuides()
	out := make([]Guide, 0, len(b))
	for _, g := range b {
		switch g.ID {
		case "overview-get-started":
			out = append(out, withCopy(g,
				[]string{"get started", "how do i start", "onboarding", "first steps"},
				"Start from Overview to see setup progress, then open each suggested area to finish configuration.",
				[]string{"Open Overview", "Review incomplete setup cards", "Open each card and finish the steps", "Return here to confirm progress"}))
		case "bills-open-close":
			out = append(out, withCopy(g,
				[]string{"close bill", "open bill", "split check", "settle bill"},
				"Use Bills to review open checks, apply payments, and close tables after service.",
				[]string{"Open the Bills tab", "Select the open check", "Review items and totals", "Take payment or mark paid", "Close the bill when settled"}))
		case "cash-register-shift":
			out = append(out, withCopy(g,
				[]string{"cash register", "open shift", "close shift", "drawer"},
				"Cash Register tracks shift open/close and cash movements for the day.",
				[]string{"Open Cash Register", "Start or select the active shift", "Record cash in/out as needed", "Close the shift at end of day"}))
		case "printers-connect":
			out = append(out, withCopy(g,
				[]string{"printer", "receipt printer", "thermal", "print bill"},
				"Connect printers under Settings → Printers, then assign roles for bills and receipts.",
				[]string{"Open Settings → Printers", "Add a printer with the wizard", "Assign bill or receipt roles", "Run a test print"}))
		case "kitchen-order-flow":
			out = append(out, withCopy(g,
				[]string{"kitchen display", "kds", "order ticket", "kitchen flow"},
				"Kitchen shows live tickets. Guest ordering follows the Kitchen+Orders toggle; staff can still enter orders when guest ordering is off.",
				[]string{"Open Kitchen", "Confirm Kitchen+Orders are enabled for guests if needed", "Watch new tickets appear", "Mark items in progress or done"}))
		case "reservations-manage":
			out = append(out, withCopy(g,
				[]string{"reservation", "book table", "waitlist", "booking"},
				"Reservations lets you enable booking, set party sizes, and manage the waitlist.",
				[]string{"Open Reservations", "Enable reservations if off", "Set party size limits", "Review upcoming bookings"}))
		case "menu-add-item":
			out = append(out, withCopy(g,
				[]string{"add menu items", "add item", "new dish", "how do i add menu", "create menu item", "agrego platos"},
				"Add dishes from the Menu tab: pick a category, create the item, set price and availability, then save.",
				[]string{"Open the Menu tab", "Select or create a category", "Click Add item", "Enter name, price, and availability", "Save the item"}))
		case "tables-create-qr":
			out = append(out, withCopy(g,
				[]string{"table qr", "create table", "qr code", "table code"},
				"Create tables and download QR codes so guests can open the digital menu.",
				[]string{"Open Tables", "Create a table", "Open row actions", "Download the QR code"}))
		case "guest-order-experience":
			out = append(out, withCopy(g,
				[]string{"how guests order", "how do guests order", "guest ordering", "diner menu", "how customers order", "what will diners see", "what diners see", "diner ui", "sage greeting"},
				"Guests scan the table QR (or open your public page). Sage greets them in the diner chat, they see the digital menu, tap items or talk with AI Waiter, send the order to the kitchen, then pay the bill in that same diner UI. Staff do not place the guest order from Overview setup.",
				[]string{"Guest scans the table QR on their phone", "Sage greets them; digital menu and AI Waiter open", "Guest adds items or asks the waiter", "Order goes to Kitchen; guest pays the bill"}))
		case "ai-waiter-configure":
			out = append(out, withCopy(g,
				[]string{"ai waiter", "configure sage", "guest chat", "whatsapp waiter"},
				"Configure AI Waiter name, priority, and channel visibility.",
				[]string{"Open AI Waiter", "Enable the service", "Set name and priority", "Save configuration"}))
		case "ai-waiter-takeover":
			out = append(out, withCopy(g,
				[]string{"pause ai", "pause the ai", "pause assistant", "stop ai", "turn off ai", "take over", "take over chat", "takeover", "resume ai", "answer the guest myself"},
				"You can stop the AI two ways. For one guest chat, open AI Waiter → Live Monitor, pick the conversation up and press Pause AI (Takeover); the AI stops replying there and you answer as staff. To stop it everywhere, switch the service off in AI Waiter → Overview & Settings — guests keep the menu but get no AI replies. I only show you where; I never toggle it for you.",
				[]string{"Open AI Waiter → Live Monitor", "Open the conversation and pick it up", "Press Pause AI (Takeover) and reply yourself", "Press Resume AI when you are done", "To stop every chat, switch the service off in Overview & Settings"}))
		case "director-boundary":
			out = append(out, withCopy(g,
				[]string{"director console", "director", "revenue analysis", "propose change"},
				"Ops Assistant can explain and navigate, but only Director Console (owner only) can propose operational changes for your review. Ops will never run analysis or apply actions for you.",
				[]string{"Open Director Console if you have access", "Ask for analysis there", "Review any proposals before applying", "Use Ops only for how-to guidance"}))
		case "marketing-create-draft":
			out = append(out, withCopy(g,
				[]string{"marketing", "social post", "campaign draft", "create draft"},
				"Marketing helps you draft campaigns. You review and publish outside Ops — this guide never posts for you.",
				[]string{"Open Marketing", "Pick a suggestion or start a draft", "Review caption and image", "Publish only from the Marketing tools when ready"}))
		case "analytics-read":
			out = append(out, withCopy(g,
				[]string{"analytics", "reports", "sales chart", "performance"},
				"Analytics shows sales and traffic trends. Use filters to narrow the date range.",
				[]string{"Open Analytics", "Choose the date range", "Review charts and metrics"}))
		case "crm-customer":
			out = append(out, withCopy(g,
				[]string{"crm", "customer", "guest profile", "tags"},
				"CRM stores customer profiles, tags, and notes for follow-up.",
				[]string{"Open CRM", "Search or open a customer", "Add tags or notes as needed"}))
		case "delivery-configure":
			out = append(out, withCopy(g,
				[]string{"delivery", "delivery zones", "drivers", "order delivery"},
				"Configure in-house or third-party delivery, zones, and fees under Delivery.",
				[]string{"Open Delivery", "Enable delivery mode", "Set zones and fees", "Save settings"}))
		case "counter-use":
			out = append(out, withCopy(g,
				[]string{"counter", "walk-in", "counter order"},
				"Counter is for walk-in and counter service orders without a table QR.",
				[]string{"Open Counter", "Start a new counter order", "Add items", "Send to kitchen or close the check"}))
		case "inventory-stock":
			out = append(out, withCopy(g,
				[]string{"inventory", "stock", "86", "out of stock"},
				"Inventory tracks stock levels and can hide items from AI recommendations when depleted.",
				[]string{"Open Inventory", "Review low-stock items", "Adjust quantities or block items"}))
		case "staff-invite":
			out = append(out, withCopy(g,
				[]string{"invite staff", "add team member", "roles", "permissions"},
				"Invite staff and assign roles from the Staff tab. Permissions control what each person can see.",
				[]string{"Open Staff", "Click invite", "Enter email and role", "Send the invitation"}))
		case "schedule-build":
			out = append(out, withCopy(g,
				[]string{"schedule", "roster", "shifts", "staff schedule"},
				"Build weekly schedules and assign shifts under Schedule.",
				[]string{"Open Schedule", "Select the week", "Add shifts for staff", "Save the schedule"}))
		case "business-page-publish":
			out = append(out, withCopy(g,
				[]string{"business page", "publish page", "landing page", "public page"},
				"Edit and publish your public business page from Business Page.",
				[]string{"Open Business Page", "Update content and design", "Preview changes", "Publish when ready"}))
		case "accounting-review":
			out = append(out, withCopy(g,
				[]string{"accounting", "ledger", "payouts", "financial review"},
				"Accounting shows ledger activity and financial summaries for reconciliation.",
				[]string{"Open Accounting", "Pick the period", "Review entries and totals"}))
		case "fiscal-setup":
			out = append(out, withCopy(g,
				[]string{"fiscal", "tax invoice", "arca", "official receipt"},
				"Fiscal settings configure official receipt issuance for paid bills in supported countries.",
				[]string{"Open Fiscal", "Complete business fiscal profile", "Connect the provider when ready", "Test issuance on a paid bill"}))
		case "plugins-connect":
			out = append(out, withCopy(g,
				[]string{"plugins", "stripe", "paypal", "payment plugin", "integrations"},
				"Connect payment and integration plugins from the Plugins catalog.",
				[]string{"Open Plugins", "Choose a plugin", "Enter configuration", "Enable when tests pass"}))
		case "settings-update":
			out = append(out, withCopy(g,
				[]string{"settings", "business settings", "hours", "address"},
				"Update business profile, hours, and preferences under Settings.",
				[]string{"Open Settings", "Edit the section you need", "Save changes"}))
		case "support-contact":
			out = append(out, withCopy(g,
				[]string{"support", "contact support", "connect me with support", "human help", "conectame con soporte", "soporte", "help from payverge"},
				"I can create a support request for the Payverge team. Tell me you want support and I will escalate — I will not change restaurant data.",
				[]string{"Say that you want to contact support", "Confirm the escalation when asked", "Watch for the confirmation that the request was filed"}))
		default:
			out = append(out, g)
		}
	}
	return applyGuidePresentation("en", out)
}

func esGuides() []Guide {
	b := baseGuides()
	out := make([]Guide, 0, len(b))
	for _, g := range b {
		switch g.ID {
		case "overview-get-started":
			out = append(out, withCopy(g,
				[]string{"primeros pasos", "cómo empezar", "configuración inicial", "progreso de configuración"},
				"En Resumen puedes ver el avance de la configuración y entrar en cada área sugerida para completar lo pendiente.",
				[]string{"Abre Resumen", "Revisa las tarjetas de configuración pendientes", "Entra en cada tarjeta y completa sus pasos", "Vuelve a Resumen para confirmar el avance"}))
		case "bills-open-close":
			out = append(out, withCopy(g,
				[]string{"abrir cuenta", "cerrar cuenta", "dividir cuenta", "cobrar mesa"},
				"En Cuentas puedes revisar consumos abiertos, registrar pagos y cerrar las mesas cuando el total esté saldado.",
				[]string{"Abre Cuentas", "Selecciona la cuenta abierta", "Revisa los productos y el total", "Registra el pago o márcala como pagada", "Cierra la cuenta cuando esté saldada"}))
		case "cash-register-shift":
			out = append(out, withCopy(g,
				[]string{"caja registradora", "abrir turno de caja", "cerrar turno de caja", "movimientos de efectivo"},
				"Caja registra la apertura y el cierre del turno, además de los movimientos de efectivo del día.",
				[]string{"Abre Caja", "Inicia o selecciona el turno activo", "Registra las entradas y salidas de efectivo necesarias", "Cierra el turno al terminar el día"}))
		case "printers-connect":
			out = append(out, withCopy(g,
				[]string{"impresora", "impresora térmica", "imprimir recibo", "conectar impresora"},
				"Conecta las impresoras desde Configuración → Impresoras y asigna su función para facturas o recibos.",
				[]string{"Entra en Configuración → Impresoras", "Agrega una impresora con el asistente", "Asigna la función de factura o recibo", "Haz una impresión de prueba"}))
		case "kitchen-order-flow":
			out = append(out, withCopy(g,
				[]string{"pantalla de cocina", "comandas de cocina", "flujo de pedidos", "pedidos en cocina"},
				"Cocina muestra las comandas en vivo. El interruptor Cocina y Pedidos controla los pedidos de clientes; el personal puede cargar pedidos aunque esté apagado.",
				[]string{"Abre Cocina", "Confirma que Cocina y Pedidos esté activo si vas a aceptar pedidos de clientes", "Observa la llegada de nuevas comandas", "Marca los productos en preparación o listos"}))
		case "reservations-manage":
			out = append(out, withCopy(g,
				[]string{"reservas", "reservar mesa", "lista de espera", "gestionar reservas"},
				"Reservaciones permite habilitar las solicitudes, definir límites de comensales y gestionar la lista de espera.",
				[]string{"Abre Reservaciones", "Activa las reservas si están deshabilitadas", "Define los límites de comensales", "Revisa las próximas reservas"}))
		case "menu-add-item":
			out = append(out, withCopy(g,
				[]string{"agregar platos", "cómo agrego platos", "nuevo plato", "añadir producto", "agregar producto al menú"},
				"Agrega platos desde Menú: elige una categoría, crea el producto, define su precio y disponibilidad, y guarda los cambios.",
				[]string{"Abre Menú", "Elige o crea una categoría", "Pulsa Agregar Elemento", "Completa el nombre, el precio y la disponibilidad", "Guarda el producto"}))
		case "tables-create-qr":
			out = append(out, withCopy(g,
				[]string{"código qr de mesa", "crear mesa", "descargar qr", "menú qr"},
				"Crea las mesas y descarga sus códigos QR para que los clientes puedan abrir el menú digital.",
				[]string{"Abre Mesas", "Crea una mesa", "Abre las acciones de esa fila", "Descarga el código QR"}))
		case "guest-order-experience":
			out = append(out, withCopy(g,
				[]string{"cómo piden los clientes", "cómo piden los comensales", "pedido de los comensales", "menú del comensal", "qué ven los comensales", "qué ven los clientes"},
				"El comensal escanea el QR de la mesa (o abre tu página). Camarero IA lo saluda en el chat, ve el menú digital, toca platos o habla con el camarero, envía el pedido a cocina y paga la cuenta en esa misma pantalla. Ops no carga ese pedido desde la guía de configuración.",
				[]string{"El comensal escanea el QR de la mesa", "Camarero IA saluda; se abre el menú digital", "Agrega platos o se lo pide al camarero", "El pedido llega a Cocina y paga la cuenta"}))
		case "ai-waiter-configure":
			out = append(out, withCopy(g,
				[]string{"mesero con inteligencia artificial", "configurar asistente", "chat para clientes", "atención por whatsapp"},
				"Configura el nombre, la prioridad y la visibilidad por canal de Camarero IA.",
				[]string{"Abre Camarero IA", "Activa el servicio", "Define el nombre y la prioridad", "Guarda la configuración"}))
		case "ai-waiter-takeover":
			out = append(out, withCopy(g,
				[]string{"pausar ia", "pausar la ia", "pausar camarero ia", "detener ia", "apagar ia", "intervenir chat", "tomar chat", "reanudar ia", "responder yo al cliente"},
				"Puedes detener la IA de dos formas. Para un solo chat, abre Camarero IA → Monitor en Vivo, toma la conversación y pulsa Pausar IA (Intervenir); la IA deja de responder ahí y contestas tú. Para detenerla en todo el local, desactiva el servicio en Camarero IA → Resumen y Configuración: los clientes siguen viendo el menú pero no reciben respuestas de IA. Yo solo te indico dónde; nunca lo cambio por ti.",
				[]string{"Abre Camarero IA → Monitor en Vivo", "Abre la conversación y tómala", "Pulsa Pausar IA (Intervenir) y responde tú", "Pulsa Reanudar IA cuando termines", "Para detener todos los chats, desactiva el servicio en Resumen y Configuración"}))
		case "director-boundary":
			out = append(out, withCopy(g,
				[]string{"director", "consola del director", "análisis de ingresos", "proponer un cambio"},
				"El Asistente Ops puede explicar y navegar, pero solo la Consola del Director (solo el propietario) puede proponer cambios operativos para que los revises. Ops nunca ejecuta análisis ni aplica acciones por ti.",
				[]string{"Abre la Consola del Director si tienes acceso", "Pide el análisis allí", "Revisa cualquier propuesta antes de aplicarla", "Usa Ops solo para obtener guías de uso"}))
		case "marketing-create-draft":
			out = append(out, withCopy(g,
				[]string{"marketing", "publicación en redes", "borrador de campaña", "crear un borrador"},
				"Marketing te ayuda a preparar campañas. Debes revisar y publicar desde sus herramientas; esta guía nunca publica por ti.",
				[]string{"Abre Marketing", "Elige una sugerencia o inicia un borrador", "Revisa el texto y la imagen", "Publica desde Marketing solo cuando esté listo"}))
		case "analytics-read":
			out = append(out, withCopy(g,
				[]string{"analíticas", "informes", "gráfico de ventas", "rendimiento"},
				"Analíticas muestra tendencias de ventas y tráfico. Usa los filtros para acotar el período.",
				[]string{"Abre Analíticas", "Elige el período", "Revisa los gráficos y las métricas"}))
		case "crm-customer":
			out = append(out, withCopy(g,
				[]string{"clientes", "perfil de cliente", "etiquetas de clientes", "notas de clientes"},
				"CRM guarda perfiles de clientes, etiquetas y notas para facilitar el seguimiento.",
				[]string{"Abre CRM", "Busca o abre un cliente", "Agrega las etiquetas o notas necesarias"}))
		case "delivery-configure":
			out = append(out, withCopy(g,
				[]string{"reparto", "zonas de entrega", "repartidores", "configurar entregas"},
				"Configura en Entrega el reparto propio o de terceros, las zonas de entrega y sus cargos.",
				[]string{"Abre Entrega", "Activa el modo de entrega", "Define las zonas y los cargos", "Guarda la configuración"}))
		case "counter-use":
			out = append(out, withCopy(g,
				[]string{"mostrador", "venta sin mesa", "pedido para llevar", "pedido en mostrador"},
				"Mostradores permite cargar pedidos presenciales sin usar el código QR de una mesa.",
				[]string{"Abre Mostradores", "Inicia un pedido de mostrador", "Agrega los productos", "Envíalo a cocina o cierra la cuenta"}))
		case "inventory-stock":
			out = append(out, withCopy(g,
				[]string{"inventario", "existencias", "sin stock", "86", "agotado", "producto agotado"},
				"Inventario controla las existencias y puede ocultar de las recomendaciones de IA los productos agotados (los platos \"86\"). Filtra por Agotado para ver la lista.",
				[]string{"Abre Inventario", "Filtra por Agotado para ver los productos 86", "Revisa también los productos con pocas existencias", "Ajusta las cantidades o bloquea los productos"}))
		case "staff-invite":
			out = append(out, withCopy(g,
				[]string{"invitar personal", "agregar integrante", "roles del equipo", "permisos del personal"},
				"Invita al personal y asigna sus roles desde Personal. Los permisos determinan qué puede ver cada integrante.",
				[]string{"Abre Personal", "Pulsa Invitar", "Completa el correo y el rol", "Envía la invitación"}))
		case "schedule-build":
			out = append(out, withCopy(g,
				[]string{"horarios", "turnos del personal", "planificar semana", "cuadrante de turnos"},
				"Organiza los horarios semanales y asigna los turnos del equipo desde Horario.",
				[]string{"Abre Horario", "Selecciona la semana", "Agrega los turnos del personal", "Guarda el horario"}))
		case "business-page-publish":
			out = append(out, withCopy(g,
				[]string{"página del negocio", "publicar página", "página pública", "editar sitio del negocio"},
				"Edita y publica la página pública de tu restaurante desde Página de Negocio.",
				[]string{"Abre Página de Negocio", "Actualiza el contenido y el diseño", "Previsualiza los cambios", "Publica cuando esté listo"}))
		case "accounting-review":
			out = append(out, withCopy(g,
				[]string{"contabilidad", "libro contable", "liquidaciones", "revisión financiera"},
				"Contabilidad muestra la actividad del libro y los resúmenes financieros para la conciliación.",
				[]string{"Abre Contabilidad", "Elige el período", "Revisa los movimientos y los totales"}))
		case "fiscal-setup":
			out = append(out, withCopy(g,
				[]string{"configuración fiscal", "factura fiscal", "arca", "comprobante oficial"},
				"La sección Facturas permite configurar la emisión de comprobantes oficiales para cuentas pagadas en los países compatibles.",
				[]string{"Abre Facturas", "Completa el perfil fiscal del negocio", "Conecta el proveedor cuando esté listo", "Prueba la emisión con una cuenta pagada"}))
		case "plugins-connect":
			out = append(out, withCopy(g,
				[]string{"plugins", "stripe", "paypal", "integración de pagos", "conectar integración"},
				"Conecta proveedores de pago y otras integraciones desde el catálogo de Plugins.",
				[]string{"Abre Plugins", "Elige un plugin", "Completa su configuración", "Actívalo cuando las pruebas resulten correctas"}))
		case "settings-update":
			out = append(out, withCopy(g,
				[]string{"configuración", "datos del negocio", "horarios de atención", "dirección del local"},
				"Actualiza el perfil del negocio, los horarios y las preferencias desde Configuración.",
				[]string{"Abre Configuración", "Edita la sección que necesitas", "Guarda los cambios"}))
		case "support-contact":
			out = append(out, withCopy(g,
				[]string{"soporte", "contactar soporte", "conéctame con soporte", "ayuda humana", "hablar con alguien"},
				"Puedo crear una solicitud de soporte para el equipo de Payverge. Dime que quieres soporte y la escalaré; no cambiaré datos del restaurante.",
				[]string{"Indica que quieres contactar con soporte", "Confirma la escalación cuando se te pida", "Espera la confirmación de que se registró la solicitud"}))
		}
	}
	return applyGuidePresentation("es", out)
}

func esARGuides() []Guide {
	es := esGuides()
	out := make([]Guide, 0, len(es))
	for _, g := range es {
		switch g.ID {
		case "overview-get-started":
			g.Answer = "En Resumen podés ver el avance de la configuración y entrar en cada área sugerida para completar lo pendiente."
			g.Steps = []string{"Abrí Resumen", "Revisá las tarjetas de configuración pendientes", "Entrá en cada tarjeta y completá sus pasos", "Volvé a Resumen para confirmar el avance"}
		case "bills-open-close":
			g.Answer = "En Cuentas podés revisar consumos abiertos, registrar pagos y cerrar las mesas cuando el total esté saldado."
			g.Steps = []string{"Abrí Cuentas", "Seleccioná la cuenta abierta", "Revisá los productos y el total", "Registrá el pago o marcala como pagada", "Cerrá la cuenta cuando esté saldada"}
		case "cash-register-shift":
			g.Steps = []string{"Abrí Caja", "Iniciá o seleccioná el turno activo", "Registrá las entradas y salidas de efectivo necesarias", "Cerrá el turno al terminar el día"}
		case "printers-connect":
			g.Answer = "Conectá las impresoras desde Configuración → Impresoras y asignales su función para facturas o recibos."
			g.Steps = []string{"Entrá en Configuración → Impresoras", "Agregá una impresora con el asistente", "Asignale la función de factura o recibo", "Hacé una impresión de prueba"}
		case "kitchen-order-flow":
			g.Steps = []string{"Abrí Cocina", "Confirmá que Cocina y Pedidos esté activo si querés aceptar pedidos de clientes", "Observá la llegada de nuevas comandas", "Marcá los productos en preparación o listos"}
		case "reservations-manage":
			g.Steps = []string{"Abrí Reservaciones", "Activá las reservas si están deshabilitadas", "Definí los límites de comensales", "Revisá las próximas reservas"}
		case "menu-add-item":
			g.Phrases = []string{"agregar platos", "cómo agrego platos", "nuevo plato", "añadir ítem", "agregar item del menú"}
			g.Answer = "Agregá platos desde Menú: elegí una categoría, creá el producto, definí su precio y disponibilidad, y guardá los cambios."
			g.Steps = []string{"Abrí Menú", "Elegí o creá una categoría", "Tocá Agregar Elemento", "Completá el nombre, el precio y la disponibilidad", "Guardá el producto"}
		case "tables-create-qr":
			g.Answer = "Creá las mesas y descargá sus códigos QR para que los clientes puedan abrir el menú digital."
			g.Steps = []string{"Abrí Mesas", "Creá una mesa", "Abrí las acciones de esa fila", "Descargá el código QR"}
		case "guest-order-experience":
			g.Phrases = []string{"cómo piden los clientes", "cómo piden los comensales", "pedido de los comensales", "menú del comensal", "qué ven los comensales", "qué ven los clientes"}
			g.Answer = "El comensal escanea el QR de la mesa (o abre tu página). Mozo IA lo saluda en el chat, ve el menú digital, toca platos o habla con el mozo, manda el pedido a cocina y paga la cuenta en esa misma pantalla. Ops no carga ese pedido desde la guía de configuración."
			g.Steps = []string{"El comensal escanea el QR de la mesa", "Mozo IA saluda; se abre el menú digital", "Agregá platos o pedíselo al mozo", "El pedido llega a Cocina y paga la cuenta"}
		case "ai-waiter-configure":
			g.Answer = "Configurá el nombre, la prioridad y la visibilidad por canal de Mozo IA."
			g.Steps = []string{"Abrí Mozo IA", "Activá el servicio", "Definí el nombre y la prioridad", "Guardá la configuración"}
		case "ai-waiter-takeover":
			g.Phrases = []string{"pausar ia", "pausar la ia", "pausar mozo ia", "detener ia", "apagar ia", "intervenir chat", "tomar chat", "reanudar ia", "responder yo al cliente"}
			g.Answer = "Podés frenar la IA de dos formas. Para un solo chat, abrí Mozo IA → Monitor en Vivo, tomá la conversación y tocá Pausar IA (Intervenir); la IA deja de responder ahí y contestás vos. Para frenarla en todo el local, desactivá el servicio en Mozo IA → Resumen y Configuración: los clientes siguen viendo el menú pero no reciben respuestas de IA. Yo solo te indico dónde; nunca lo cambio por vos."
			g.Steps = []string{"Abrí Mozo IA → Monitor en Vivo", "Abrí la conversación y tomala", "Tocá Pausar IA (Intervenir) y respondé vos", "Tocá Reanudar IA cuando termines", "Para frenar todos los chats, desactivá el servicio en Resumen y Configuración"}
		case "director-boundary":
			g.Answer = "El Asistente Ops puede explicar y navegar, pero solo la Consola del Director (solo el propietario) puede proponer cambios operativos para que los revises. Ops nunca ejecuta análisis ni aplica acciones por vos."
			g.Steps = []string{"Abrí la Consola del Director si tenés acceso", "Pedí el análisis allí", "Revisá cualquier propuesta antes de aplicarla", "Usá Ops solo para obtener guías de uso"}
		case "marketing-create-draft":
			g.Answer = "Marketing te ayuda a preparar campañas. Tenés que revisar y publicar desde sus herramientas; esta guía nunca publica por vos."
			g.Steps = []string{"Abrí Marketing", "Elegí una sugerencia o iniciá un borrador", "Revisá el texto y la imagen", "Publicá desde Marketing solo cuando esté listo"}
		case "analytics-read":
			g.Answer = "Analíticas muestra tendencias de ventas y tráfico. Usá los filtros para acotar el período."
			g.Steps = []string{"Abrí Analíticas", "Elegí el período", "Revisá los gráficos y las métricas"}
		case "crm-customer":
			g.Steps = []string{"Abrí CRM", "Buscá o abrí un cliente", "Agregá las etiquetas o notas necesarias"}
		case "delivery-configure":
			g.Answer = "Configurá en Entrega el reparto propio o de terceros, las zonas de entrega y sus cargos."
			g.Steps = []string{"Abrí Entrega", "Activá el modo de entrega", "Definí las zonas y los cargos", "Guardá la configuración"}
		case "counter-use":
			g.Steps = []string{"Abrí Mostradores", "Iniciá un pedido de mostrador", "Agregá los productos", "Envialo a cocina o cerrá la cuenta"}
		case "inventory-stock":
			g.Answer = "Inventario controla las existencias y puede ocultar de las recomendaciones de IA los productos agotados (los platos \"86\"). Filtrá por Agotado para ver la lista."
			g.Steps = []string{"Abrí Inventario", "Filtrá por Agotado para ver los productos 86", "Revisá también los productos con pocas existencias", "Ajustá las cantidades o bloqueá los productos"}
		case "staff-invite":
			g.Answer = "Invitá al personal y asignale sus roles desde Personal. Los permisos determinan qué puede ver cada integrante."
			g.Steps = []string{"Abrí Personal", "Tocá Invitar", "Completá el correo y el rol", "Enviá la invitación"}
		case "schedule-build":
			g.Answer = "Organizá los horarios semanales y asigná los turnos del equipo desde Horario."
			g.Steps = []string{"Abrí Horario", "Seleccioná la semana", "Agregá los turnos del personal", "Guardá el horario"}
		case "business-page-publish":
			g.Answer = "Editá y publicá la página pública de tu restaurante desde Página de Negocio."
			g.Steps = []string{"Abrí Página de Negocio", "Actualizá el contenido y el diseño", "Previsualizá los cambios", "Publicá cuando esté listo"}
		case "accounting-review":
			g.Steps = []string{"Abrí Contabilidad", "Elegí el período", "Revisá los movimientos y los totales"}
		case "fiscal-setup":
			g.Steps = []string{"Abrí Facturas", "Completá el perfil fiscal del negocio", "Conectá el proveedor cuando esté listo", "Probá la emisión con una cuenta pagada"}
		case "plugins-connect":
			g.Answer = "Conectá proveedores de pago y otras integraciones desde el catálogo de Plugins."
			g.Steps = []string{"Abrí Plugins", "Elegí un plugin", "Completá su configuración", "Activalo cuando las pruebas resulten correctas"}
		case "settings-update":
			g.Answer = "Actualizá el perfil del negocio, los horarios y las preferencias desde Configuración."
			g.Steps = []string{"Abrí Configuración", "Editá la sección que necesitás", "Guardá los cambios"}
		case "support-contact":
			g.Phrases = []string{"soporte", "contactar soporte", "conectame con soporte", "ayuda humana", "hablar con alguien"}
			g.Answer = "Puedo crear una solicitud de soporte para el equipo de Payverge. Decime que querés soporte y la escalo; no voy a cambiar datos del restaurante."
			g.Steps = []string{"Indicá que querés contactar con soporte", "Confirmá la escalación cuando te lo pida", "Esperá la confirmación de que se registró la solicitud"}
		}
		out = append(out, g)
	}
	return applyGuidePresentation("es-AR", out)
}
