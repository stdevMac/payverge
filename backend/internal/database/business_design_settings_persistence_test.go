package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// hero_layout and section_density are editor-exposed and read by the public
// page; they must round-trip through the embedded design settings instead of
// being silently dropped on bind/save.
func TestBusinessDesignSettings_HeroLayoutAndDensityRoundTrip(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&Business{}))

	biz := Business{
		BusinessId: "design-roundtrip-biz",
		Name:       "Design Roundtrip Biz",
		DesignSettings: BusinessDesignSettings{
			PrimaryColor:   "#1a6b6a",
			HeroLayout:     "split-left",
			SectionDensity: "compact",
		},
	}
	require.NoError(t, gdb.Create(&biz).Error)

	var reloaded Business
	require.NoError(t, gdb.First(&reloaded, biz.ID).Error)
	assert.Equal(t, "split-left", reloaded.DesignSettings.HeroLayout)
	assert.Equal(t, "compact", reloaded.DesignSettings.SectionDensity)
}
