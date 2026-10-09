package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// saveAssocSQLRecorder captures SQL so association-upsert regressions can be
// asserted without a live Postgres (PG-9 / T-1 class).
type saveAssocSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *saveAssocSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func writesAgainstTable(statements []string, table string) []string {
	var hits []string
	for _, statement := range statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		for _, prefix := range []string{
			"insert into `" + table + "`",
			"insert into \"" + table + "\"",
			"insert into " + table,
			"update `" + table + "`",
			"update \"" + table + "\"",
			"update " + table,
		} {
			if strings.HasPrefix(normalized, prefix) {
				hits = append(hits, statement)
				break
			}
		}
	}
	return hits
}

func setupSaveAssocTestDB(t *testing.T, rec *saveAssocSQLRecorder, models ...interface{}) {
	t.Helper()
	cfg := &gorm.Config{}
	if rec != nil {
		cfg.Logger = rec
	}
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:save_assoc_%s?mode=memory&cache=shared", t.Name())), cfg)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	db = gormDB
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
}

// TestUpdateInventoryItem_SaveMustNotWriteBusiness guards the T-1 class on
// InventoryItem (belongs-to Business). A partial Business preload must not be
// upserted when the item is saved — same defect class as PG-9 reservations.
func TestUpdateInventoryItem_SaveMustNotWriteBusiness(t *testing.T) {
	rec := &saveAssocSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupSaveAssocTestDB(t, rec, &Business{}, &InventoryItem{}, &InventoryMovement{})

	biz := &Business{
		BusinessId:     fmt.Sprintf("inv-assoc-%d", time.Now().UnixNano()),
		OwnerAddress:   "0xINVOWNER",
		Name:           "Inv Assoc Resto",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(biz).Error)

	item := &InventoryItem{
		BusinessID:      biz.ID,
		Name:            "Flour",
		Unit:            "kg",
		CurrentQuantity: 10,
		IsActive:        true,
	}
	require.NoError(t, db.Create(item).Error)

	// Attach a partial Business projection — the PG-9 shape (ID set, owner empty).
	var loaded InventoryItem
	require.NoError(t, db.First(&loaded, item.ID).Error)
	loaded.Business = Business{ID: biz.ID, Name: "partial-only"}
	require.Empty(t, loaded.Business.OwnerAddress)
	previous := loaded

	loaded.Name = "Flour (updated)"
	rec.statements = nil
	require.NoError(t, UpdateInventoryItem(&loaded, previous, "test@example.com"))

	// UpdateInventoryItem advances guest-orderability revision via a narrow
	// UPDATE of businesses.updated_at — that is intentional. The defect class
	// is association upsert: INSERT (or broad UPDATE) of the partial Business
	// preload. Forbid those shapes.
	for _, stmt := range rec.statements {
		n := strings.ToLower(strings.TrimSpace(stmt))
		if strings.HasPrefix(n, "insert into") && strings.Contains(n, "businesses") {
			t.Fatalf("UpdateInventoryItem must not INSERT businesses (association upsert): %s", stmt)
		}
		if strings.HasPrefix(n, "update") && strings.Contains(n, "businesses") {
			if strings.Contains(n, "owner_address") || strings.Contains(n, "name") || strings.Contains(n, "settlement") {
				t.Fatalf("UpdateInventoryItem must not upsert partial Business columns: %s", stmt)
			}
		}
	}

	var reloadedBiz Business
	require.NoError(t, db.First(&reloadedBiz, biz.ID).Error)
	require.Equal(t, "0xINVOWNER", reloadedBiz.OwnerAddress,
		"business owner must survive inventory Save untouched")
	require.Equal(t, "Inv Assoc Resto", reloadedBiz.Name,
		"business name must not be overwritten by partial preload")

	var reloadedItem InventoryItem
	require.NoError(t, db.First(&reloadedItem, item.ID).Error)
	require.Equal(t, "Flour (updated)", reloadedItem.Name)
}

// TestUpdateExtractionJob_SaveMustNotWriteAssociations guards MenuExtractionJob
// Saves after GetExtractionJobByID (which Preload-s Images). Without Omit,
// GORM can upsert the Business belongs-to and/or cascade Images.
func TestUpdateExtractionJob_SaveMustNotWriteAssociations(t *testing.T) {
	rec := &saveAssocSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupSaveAssocTestDB(t, rec, &Business{}, &MenuExtractionJob{}, &MenuExtractionImage{})

	biz := &Business{
		BusinessId:     fmt.Sprintf("extract-assoc-%d", time.Now().UnixNano()),
		OwnerAddress:   "0xEXTRACTOWNER",
		Name:           "Extract Assoc Resto",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(biz).Error)

	job := &MenuExtractionJob{
		BusinessID: biz.ID,
		Status:     ExtractionStatusPending,
		ImageCount: 1,
	}
	require.NoError(t, db.Create(job).Error)
	require.NoError(t, db.Create(&MenuExtractionImage{
		JobID:     job.ID,
		PageOrder: 0,
		FilePath:  "s3://example/page0.png",
	}).Error)

	// Real read path: Preload Images (and attach a partial Business).
	loaded, err := GetExtractionJobByID(job.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Images, 1)
	loaded.Business = Business{ID: biz.ID, Name: "partial-only"}
	require.Empty(t, loaded.Business.OwnerAddress)

	loaded.Status = ExtractionStatusCompleted
	loaded.ExtractedMenu = `{"categories":[]}`
	rec.statements = nil
	require.NoError(t, UpdateExtractionJob(loaded))

	for _, table := range []string{"businesses", "menu_extraction_images"} {
		require.Emptyf(t, writesAgainstTable(rec.statements, table),
			"UpdateExtractionJob must not write %s", table)
	}

	var reloadedBiz Business
	require.NoError(t, db.First(&reloadedBiz, biz.ID).Error)
	require.Equal(t, "0xEXTRACTOWNER", reloadedBiz.OwnerAddress)

	var reloadedJob MenuExtractionJob
	require.NoError(t, db.First(&reloadedJob, job.ID).Error)
	require.Equal(t, ExtractionStatusCompleted, reloadedJob.Status)
	require.Equal(t, `{"categories":[]}`, reloadedJob.ExtractedMenu)

	var imgCount int64
	require.NoError(t, db.Model(&MenuExtractionImage{}).Where("job_id = ?", job.ID).Count(&imgCount).Error)
	require.Equal(t, int64(1), imgCount, "images must not be deleted/rewritten by job Save")
}
