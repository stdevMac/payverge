package demo

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

// seedAssets embeds the demo food/venue photography that the seeded menus and
// business profiles reference through demoAssetURL (see menuImages and the
// profile Hero URLs). Shipping the images inside the binary makes the demo
// self-healing: EnsureSeedAssets uploads any missing object into the configured
// public store at startup (local volume or S3 bucket), so every environment
// that runs the backend serves the assets without an out-of-band upload step.
//
//go:embed assets/carta assets/promos assets/venues
var seedAssets embed.FS

// seedAssetPrefix is the store key prefix the seeded URLs live under:
// s3.PublicURL("demo-arg/assets/<carta|promos|venues>/<file>.jpg").
const seedAssetPrefix = "demo-arg/assets"

// seedAssetKeys lists the bucket keys for every embedded seed asset.
func seedAssetKeys() ([]string, error) {
	var keys []string
	err := fs.WalkDir(seedAssets, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		keys = append(keys, seedAssetPrefix+"/"+strings.TrimPrefix(p, "assets/"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk embedded demo assets: %w", err)
	}
	return keys, nil
}

// seedAssetCheck matches s3.ObjectExists; seedAssetUpload matches s3.UploadBytes.
// Both are injectable so tests never touch a live bucket.
type seedAssetCheck func(key string) (bool, error)

// EnsureSeedAssets uploads every embedded demo asset that is missing from the
// configured public bucket. Existing objects are left untouched, so a manually
// curated replacement in the bucket survives restarts. Errors are aggregated
// and returned (callers log-and-continue: a broken bucket must not block
// startup or demo seeding).
func EnsureSeedAssets(exists seedAssetCheck, upload uploadFunc) (uploaded int, err error) {
	if exists == nil {
		exists = s3.ObjectExists
	}
	if upload == nil {
		upload = s3.UploadBytes
	}

	keys, err := seedAssetKeys()
	if err != nil {
		return 0, err
	}

	var problems []string
	for _, key := range keys {
		present, checkErr := exists(key)
		if checkErr != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", key, checkErr))
			continue
		}
		if present {
			continue
		}
		data, readErr := seedAssets.ReadFile("assets/" + strings.TrimPrefix(key, seedAssetPrefix+"/"))
		if readErr != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", key, readErr))
			continue
		}
		if _, upErr := upload(data, path.Base(key), path.Dir(key), "image/jpeg"); upErr != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", key, upErr))
			continue
		}
		uploaded++
	}
	if len(problems) > 0 {
		return uploaded, fmt.Errorf("ensure demo seed assets: %s", strings.Join(problems, "; "))
	}
	return uploaded, nil
}
