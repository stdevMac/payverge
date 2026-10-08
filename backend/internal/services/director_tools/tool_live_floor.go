package director_tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

// LiveFloorTool is a READ of the live dining room: table occupancy, open
// checks, waiting service calls, and the open cash drawer. It never
// dispatches staff or closes the register.
type LiveFloorTool struct{}

func (t *LiveFloorTool) Name() string { return "get_live_floor" }

func (t *LiveFloorTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando el salón en vivo"
	case "fr":
		return "Lecture du service en cours"
	case "ar":
		return "قراءة حالة الصالة"
	default:
		return "Reading the live floor"
	}
}

func (t *LiveFloorTool) Description() string {
	return "Returns live table occupancy (free/occupied/reserved), open-check totals, waiting service calls, and the open cash drawer. Call for 'which tables are free', 'why is a table waiting', 'send a waiter', 'how much is open tonight', or 'close the cash register' (report only — this tool never dispatches or closes). Use get_kitchen_status for tickets and kitchen staff."
}

func (t *LiveFloorTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}

func (t *LiveFloorTool) Run(_ context.Context, _ map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_live_floor: nil DB in tool env")
	}

	rows, err := database.GetTablesWithStatus(env.BusinessID, false)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_live_floor: tables: %w", err)
	}

	free := make([]string, 0)
	occupied := make([]map[string]any, 0)
	reserved := 0
	openChecks := 0
	var openTotalCents int64
	for _, row := range rows {
		tbl, _ := row["table"].(database.Table)
		status, _ := row["status"].(string)
		name := strings.TrimSpace(tbl.Name)
		if name == "" {
			name = tbl.TableCode
		}
		switch status {
		case "available":
			free = append(free, name)
		case "reserved":
			reserved++
		case "occupied":
			bills, _ := row["active_bills"].([]database.Bill)
			var cents int64
			for _, b := range bills {
				cents += b.TotalAmount
				openChecks++
			}
			openTotalCents += cents
			server, _ := row["active_bill_server_name"].(string)
			occupied = append(occupied, map[string]any{
				"table":      name,
				"open_check": centsToDollars(cents),
				"server":     server,
				"open_bills": len(bills),
			})
		}
	}

	calls := listOpenServiceCalls(env.DB, env.BusinessID, rows)
	drawer := readOpenCashDrawer(env.DB, env.BusinessID)

	spanish := strings.HasPrefix(strings.ToLower(strings.ReplaceAll(env.Locale, "_", "-")), "es")
	summary := liveFloorSummary(spanish, len(rows), len(free), len(occupied), reserved, openChecks, openTotalCents, len(calls), drawer)

	data := map[string]any{
		"tables_total":      len(rows),
		"available":         len(free),
		"occupied":          len(occupied),
		"reserved":          reserved,
		"free_tables":       free,
		"occupied_tables":   occupied,
		"open_checks":       openChecks,
		"open_checks_total": centsToDollars(openTotalCents),
		"service_calls":     calls,
		"cash_drawer":       drawer,
	}
	return ToolResult{Summary: summary, Data: data}, nil
}

func liveFloorSummary(spanish bool, total, free, occupied, reserved, checks int, checkCents int64, waiting int, drawer map[string]any) string {
	openCash := false
	expected := 0.0
	if drawer != nil {
		openCash, _ = drawer["open"].(bool)
		expected, _ = drawer["expected_cash"].(float64)
	}
	if spanish {
		s := fmt.Sprintf("%d mesas: %d libres, %d ocupadas, %d reservadas. %d cuentas abiertas por $%s.",
			total, free, occupied, reserved, checks, humanizeMoney(centsToDollars(checkCents)))
		if waiting > 0 {
			s += fmt.Sprintf(" %d llamados de servicio esperando.", waiting)
		}
		if openCash {
			s += fmt.Sprintf(" Caja abierta (esperado $%s).", humanizeMoney(expected))
		} else {
			s += " No hay caja abierta."
		}
		return s
	}
	s := fmt.Sprintf("%d tables: %d free, %d occupied, %d reserved. %d open checks totaling $%s.",
		total, free, occupied, reserved, checks, humanizeMoney(centsToDollars(checkCents)))
	if waiting > 0 {
		s += fmt.Sprintf(" %d service calls waiting.", waiting)
	}
	if openCash {
		s += fmt.Sprintf(" Cash drawer is open (expected $%s).", humanizeMoney(expected))
	} else {
		s += " No cash drawer is open."
	}
	return s
}

func listOpenServiceCalls(db *database.DB, businessID uint, floor []map[string]interface{}) []map[string]any {
	out := make([]map[string]any, 0)
	if db == nil {
		return out
	}
	nameByID := map[int64]string{}
	for _, row := range floor {
		tbl, _ := row["table"].(database.Table)
		name := strings.TrimSpace(tbl.Name)
		if name == "" {
			name = tbl.TableCode
		}
		nameByID[int64(tbl.ID)] = name
	}
	now := time.Now()
	var alerts []database.OperationalAlert
	if err := db.GetGorm().
		Where("business_id = ? AND alert_type = ? AND status IN ?",
			businessID, database.OperationalAlertTypeServiceCall,
			[]database.OperationalAlertStatus{
				database.OperationalAlertStatusOpen,
				database.OperationalAlertStatusClaimed,
			}).
		Order("created_at ASC").
		Find(&alerts).Error; err != nil {
		return out
	}
	billOpened := map[int64]time.Time{}
	for _, row := range floor {
		bills, _ := row["active_bills"].([]database.Bill)
		for _, bill := range bills {
			id := int64(bill.TableID)
			if existing, ok := billOpened[id]; !ok || bill.CreatedAt.After(existing) {
				billOpened[id] = bill.CreatedAt
			}
		}
	}
	for _, a := range alerts {
		reason := operational_alerts.ServiceCallReason(a.Metadata)
		if operational_alerts.IsStaleServiceCall(a.LastEventAt, now) &&
			!operational_alerts.ServiceCallTTLExempt(a.Status, reason) {
			created, ok := billOpened[a.ResourceID]
			if !operational_alerts.SameSeatingCheckPlease(reason, a.LastEventAt, created, ok) {
				continue
			}
		}
		start := a.CreatedAt
		if !a.LastEventAt.IsZero() && a.LastEventAt.Before(start) {
			start = a.LastEventAt
		} else if !a.LastEventAt.IsZero() {
			// Prefer LastEventAt when it carries the original raise time.
			if a.LastEventAt.Before(now) {
				start = a.LastEventAt
			}
		}
		wait := int(now.Sub(start).Minutes())
		if wait < 0 {
			wait = 0
		}
		tableName := nameByID[a.ResourceID]
		if tableName == "" {
			tableName = fmt.Sprintf("%d", a.ResourceID)
		}
		out = append(out, map[string]any{
			"table":        tableName,
			"title":        a.Title,
			"claimed_by":   a.ClaimedByName,
			"wait_minutes": wait,
		})
	}
	return out
}

func readOpenCashDrawer(db *database.DB, businessID uint) map[string]any {
	empty := map[string]any{"open": false, "expected_cash": 0.0}
	if db == nil {
		return empty
	}
	var session database.CashRegisterSession
	err := db.GetGorm().
		Where("business_id = ? AND status = ?", businessID, database.CashRegisterSessionStatusOpen).
		First(&session).Error
	if err != nil {
		return empty
	}
	return map[string]any{
		"open":          true,
		"expected_cash": centsToDollars(session.ExpectedCashCents),
		"opening_float": centsToDollars(session.OpeningFloatCents),
		"cash_sales":    centsToDollars(session.CashSalesCents),
	}
}

func centsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}
