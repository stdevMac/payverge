package genesis

import _ "embed"

//go:embed current_schema.sql
var SchemaSQL string

//go:embed version.json
var VersionJSON []byte
