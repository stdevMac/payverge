package services

import (
	"fmt"
	"regexp"
	"strings"
)

// groundOverviewSalesIfNeeded corrects false "payments are off" / "not enough
// data" answers when Overview already shows money. It only replaces the
// answer with tonight's sales when the owner asked for tonight/today sales.
// Best-seller and table-wait questions keep their own domain — dumping
// $18.04 as if that were the best seller or the wait reason is a second lie (#730).
func (s *DirectorConsoleService) groundOverviewSalesIfNeeded(question, locale string, businessID uint, resp DirectorStructuredResponse, payload *directorContext) DirectorStructuredResponse {
	if payload == nil {
		return resp
	}
	hasSales := payload.Metrics.TodayRevenue > 0 || payload.Metrics.ActiveBills > 0
	if !hasSales {
		return resp
	}
	falseOff := directorLooksLikePaymentsDisabled(resp)
	noData := directorLooksLikeInsufficientData(resp)
	if !falseOff && !noData {
		return resp
	}

	if directorWantsTonightSales(question) {
		return s.buildOverviewSalesResponse(businessID, locale, payload)
	}
	if directorWantsTableWait(question) && (falseOff || noData) {
		return s.buildLiveFloorWaitResponse(businessID, locale, question, payload)
	}
	if falseOff {
		return scrubFalsePaymentsDisabledClaim(resp, locale)
	}
	return resp
}

func directorLooksLikePaymentsDisabled(resp DirectorStructuredResponse) bool {
	blob := strings.ToLower(directorProseBlob(resp))
	for _, p := range []string{
		"payments are not enabled",
		"payment system is not",
		"payments system is not",
		"payments not enabled",
		"payments aren't enabled",
		"payments are not connected",
		"payments not connected",
		"sistema de pagos no",
		"pagos no está habilit",
		"pagos no esta habilit",
		"pagos no están habilit",
		"pagos no estan habilit",
		"no se pueden registrar ventas",
		"no sales can be recorded",
	} {
		if strings.Contains(blob, p) {
			return true
		}
	}
	return false
}

func directorWantsTonightSales(question string) bool {
	q := strings.ToLower(question)
	for _, kw := range []string{
		"how much did we sell", "how much we sold", "how much have we sold",
		"sold tonight", "sell tonight", "sold today", "sell today",
		"today's sales", "todays sales", "tonight's sales", "tonights sales",
		"ventas de hoy", "ventas hoy", "ventas de esta noche",
		"cuánto vend", "cuanto vend",
	} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

var directorTableWaitRe = regexp.MustCompile(`(?i)\b(?:table|mesa)\s*([0-9]+|[a-z][a-z0-9-]*)`)

func directorWantsTableWait(question string) bool {
	q := strings.ToLower(question)
	if directorTableWaitRe.FindStringSubmatch(q) == nil {
		return false
	}
	for _, kw := range []string{"wait", "waiting", "esper", "por qué", "por que", "why"} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func directorAskedTableName(question string) string {
	m := directorTableWaitRe.FindStringSubmatch(question)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func liveOpsOpenChecks(live map[string]interface{}) (int, float64) {
	if live == nil {
		return 0, 0
	}
	return directorAsInt(live["open_checks"]), directorAsFloat(live["open_checks_total"])
}

func (s *DirectorConsoleService) buildOverviewSalesResponse(businessID uint, locale string, payload *directorContext) DirectorStructuredResponse {
	spanish := directorUsesSpanishCopy(locale)
	// #870: the venue's own currency, never a bare "$". An Argentine floor
	// reading "$40600.00" for ARS 40.600 is a wrong number to an owner.
	currency := directorPayloadCurrency(payload)
	collected := payload.Metrics.TodayCollected
	floor := payload.Metrics.TodayFloorRemaining
	active := payload.Metrics.ActiveBills
	linkBills := buildTabDeepLink(fmt.Sprintf("%d", businessID), "bills")
	linkOverview := buildTabDeepLink(fmt.Sprintf("%d", businessID), "overview")

	openChecks, openTotal := liveOpsOpenChecks(payload.LiveOps)
	if openChecks == 0 {
		openChecks = active
	}
	if openTotal == 0 {
		openTotal = floor
	}

	if spanish {
		return DirectorStructuredResponse{
			Summary: fmt.Sprintf(
				"Cobrado hoy: %s. Restante en cuentas abiertas: %s. Hay %d cuentas activas.",
				directorMoney(currency, collected), directorMoney(currency, floor), active,
			),
			Diagnosis: "Estas cifras son las mismas que Overview: cobrado hoy son pagos reconocidos; el restante en cheques abiertos no es venta cobrada.",
			Evidence: []string{
				fmt.Sprintf("Cobrado hoy: %s.", directorMoney(currency, collected)),
				fmt.Sprintf("Restante en piso: %s (no es venta de hoy).", directorMoney(currency, floor)),
				fmt.Sprintf("Cuentas activas: %d. Cheques abiertos: %d por %s.", active, openChecks, directorMoney(currency, openTotal)),
			},
			ActionPlan: []DirectorAction{
				{Title: "Ver Overview", Description: "Lo cobrado hoy y las cuentas activas están en Overview.", DeepLink: linkOverview, Priority: "high"},
				{Title: "Abrir Cuentas", Description: "Revisá las cuentas abiertas o parciales en el piso.", DeepLink: linkBills, Priority: "medium"},
			},
			ExpectedImpact: fmt.Sprintf("Hay %s cobrado hoy y %s restante en el piso.", directorMoney(currency, collected), directorMoney(currency, floor)),
			FollowUps: []string{
				"¿Querés el desglose por mesa?",
				"¿Cerramos alguna cuenta abierta?",
			},
		}
	}

	return DirectorStructuredResponse{
		Summary: fmt.Sprintf(
			"Collected today: %s. Remaining on open checks: %s. There are %d active bills.",
			directorMoney(currency, collected), directorMoney(currency, floor), active,
		),
		Diagnosis: "These figures match Overview: collected today is recognized payments; remaining on live open or partial checks is not sold as today's sales.",
		Evidence: []string{
			fmt.Sprintf("Collected today: %s.", directorMoney(currency, collected)),
			fmt.Sprintf("Floor remaining: %s (not today's sales).", directorMoney(currency, floor)),
			fmt.Sprintf("Active bills: %d. Open checks: %d totaling %s.", active, openChecks, directorMoney(currency, openTotal)),
		},
		ActionPlan: []DirectorAction{
			{Title: "Open Overview", Description: "Collected today and active bills live on Overview.", DeepLink: linkOverview, Priority: "high"},
			{Title: "Review Bills", Description: "Inspect open or partial checks on the floor.", DeepLink: linkBills, Priority: "medium"},
		},
		ExpectedImpact: fmt.Sprintf("Tonight has %s collected and %s remaining on the floor.", directorMoney(currency, collected), directorMoney(currency, floor)),
		FollowUps: []string{
			"Want the breakdown by table?",
			"Should we close any open checks?",
		},
	}
}

func (s *DirectorConsoleService) buildLiveFloorWaitResponse(businessID uint, locale string, question string, payload *directorContext) DirectorStructuredResponse {
	spanish := directorUsesSpanishCopy(locale)
	table := directorAskedTableName(question)
	linkTables := buildTabDeepLink(fmt.Sprintf("%d", businessID), "tables")
	waitMin, callTitle, occupied, openCheck := liveOpsTableWait(payload.LiveOps, table)
	openChecks, _ := liveOpsOpenChecks(payload.LiveOps)
	if openChecks == 0 {
		openChecks = payload.Metrics.ActiveBills
	}

	if spanish {
		summary := "Los pagos están funcionando; el salón en vivo no depende de un plugin de cobro."
		diagnosis := "No veo un llamado de servicio para esa mesa en el piso en vivo. Revisá Mesas."
		if table != "" && waitMin >= 0 {
			summary = fmt.Sprintf("La mesa %s lleva %d minutos esperando.", table, waitMin)
			if callTitle != "" {
				diagnosis = fmt.Sprintf("Hay un llamado de servicio (%s). Pagos habilitados no es el motivo.", callTitle)
			} else {
				diagnosis = "Hay un llamado de servicio en el piso. Pagos habilitados no es el motivo."
			}
		} else if table != "" && occupied {
			summary = fmt.Sprintf("La mesa %s está ocupada.", table)
			if openCheck > 0 {
				diagnosis = fmt.Sprintf("Tiene una cuenta abierta de %s. No hay un llamado de espera en el piso en vivo.", directorMoney(directorPayloadCurrency(payload), openCheck))
			} else {
				diagnosis = "Está ocupada. No hay un llamado de espera en el piso en vivo."
			}
		}
		return DirectorStructuredResponse{
			Summary:   summary,
			Diagnosis: diagnosis,
			Evidence: []string{
				fmt.Sprintf("Cuentas activas en el piso: %d.", openChecks),
			},
			ActionPlan: []DirectorAction{
				{Title: "Abrir Mesas", Description: "El salón en vivo y los llamados de servicio están en Mesas.", DeepLink: linkTables, Priority: "high"},
			},
			ExpectedImpact: "Atender el llamado en el piso reduce la espera.",
			FollowUps:      []string{"¿Querés ver las otras mesas ocupadas?"},
		}
	}

	summary := "Payments are working; the live floor does not depend on a payment plugin."
	diagnosis := "I do not see a waiting service call for that table on the live floor. Check Tables."
	if table != "" && waitMin >= 0 {
		summary = fmt.Sprintf("Table %s has been waiting %d minutes.", table, waitMin)
		if callTitle != "" {
			diagnosis = fmt.Sprintf("There is a service call (%s). Payments being enabled is not the reason.", callTitle)
		} else {
			diagnosis = "There is a service call on the floor. Payments being enabled is not the reason."
		}
	} else if table != "" && occupied {
		summary = fmt.Sprintf("Table %s is occupied.", table)
		if openCheck > 0 {
			diagnosis = fmt.Sprintf("It has an open check of %s. No waiting service call is on the live floor.", directorMoney(directorPayloadCurrency(payload), openCheck))
		} else {
			diagnosis = "It is occupied. No waiting service call is on the live floor."
		}
	}
	return DirectorStructuredResponse{
		Summary:   summary,
		Diagnosis: diagnosis,
		Evidence: []string{
			fmt.Sprintf("Active bills on the floor: %d.", openChecks),
		},
		ActionPlan: []DirectorAction{
			{Title: "Open Tables", Description: "The live floor and service calls live on Tables.", DeepLink: linkTables, Priority: "high"},
		},
		ExpectedImpact: "Clearing the service call shortens the wait.",
		FollowUps:      []string{"Want the other occupied tables?"},
	}
}

// liveOpsTableWait returns wait minutes (>=0 when a service call matches),
// the call title, whether the table is occupied, and its open-check dollars.
func liveOpsTableWait(live map[string]interface{}, table string) (waitMin int, title string, occupied bool, openCheck float64) {
	waitMin = -1
	if live == nil || table == "" {
		return waitMin, "", false, 0
	}
	want := strings.ToLower(strings.TrimSpace(table))
	for _, raw := range directorMapSlice(live["service_calls"]) {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw["table"])))
		if name != want && !strings.HasSuffix(name, want) && !strings.HasPrefix(name, want) {
			continue
		}
		waitMin = directorAsInt(raw["wait_minutes"])
		title = strings.TrimSpace(fmt.Sprint(raw["title"]))
		if title == "<nil>" {
			title = ""
		}
		break
	}
	for _, raw := range directorMapSlice(live["occupied_tables"]) {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw["table"])))
		if name != want && !strings.HasSuffix(name, want) && !strings.HasPrefix(name, want) {
			continue
		}
		occupied = true
		openCheck = directorAsFloat(raw["open_check"])
		break
	}
	return waitMin, title, occupied, openCheck
}

func directorMapSlice(v interface{}) []map[string]any {
	out := make([]map[string]any, 0)
	switch rows := v.(type) {
	case []map[string]any:
		return rows
	case []interface{}:
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func scrubFalsePaymentsDisabledClaim(resp DirectorStructuredResponse, locale string) DirectorStructuredResponse {
	resp.Summary = scrubPaymentsDisabledPhrase(resp.Summary)
	resp.Diagnosis = scrubPaymentsDisabledPhrase(resp.Diagnosis)
	resp.ExpectedImpact = scrubPaymentsDisabledPhrase(resp.ExpectedImpact)
	for i := range resp.Evidence {
		resp.Evidence[i] = scrubPaymentsDisabledPhrase(resp.Evidence[i])
	}
	cleanActions := make([]DirectorAction, 0, len(resp.ActionPlan))
	for _, a := range resp.ActionPlan {
		blob := strings.ToLower(a.Title + " " + a.Description)
		if strings.Contains(blob, "connect payment") ||
			(strings.Contains(blob, "conect") && strings.Contains(blob, "pago")) {
			continue
		}
		a.Title = scrubPaymentsDisabledPhrase(a.Title)
		a.Description = scrubPaymentsDisabledPhrase(a.Description)
		cleanActions = append(cleanActions, a)
	}
	resp.ActionPlan = cleanActions
	if strings.TrimSpace(resp.Summary) == "" || directorLooksLikePaymentsDisabled(resp) {
		if directorUsesSpanishCopy(locale) {
			resp.Summary = "Los pagos están funcionando: hay cuentas abiertas y ventas de hoy. Eso no responde esta pregunta."
			if strings.TrimSpace(resp.Diagnosis) == "" {
				resp.Diagnosis = "No trates un piso con cuentas abiertas como un local sin cobro."
			}
		} else {
			resp.Summary = "Payments are working: this venue has open bills and today's sales. That does not answer this question."
			if strings.TrimSpace(resp.Diagnosis) == "" {
				resp.Diagnosis = "Do not treat a floor with open bills as a payments-disabled venue."
			}
		}
	}
	return resp
}

var paymentsDisabledPhraseRe = regexp.MustCompile(`(?i)(` +
	`payments? (?:system )?is not enabled` +
	`|payments? are not enabled` +
	`|payments? (?:are )?not connected` +
	`|sistema de pagos no est[aá] habilitado` +
	`|pagos no est[aá]n? habilitad[oa]s?` +
	`|no se pueden registrar ventas` +
	`|no sales can be recorded` +
	`)[^.!?]*[.!?]?`)

func scrubPaymentsDisabledPhrase(s string) string {
	cleaned := strings.TrimSpace(paymentsDisabledPhraseRe.ReplaceAllString(s, " "))
	return strings.Join(strings.Fields(cleaned), " ")
}
