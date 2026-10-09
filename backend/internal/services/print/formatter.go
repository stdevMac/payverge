package print

// FormatterOutput is what every formatter returns. Sprint 1 only fills HTML.
type FormatterOutput struct {
	HTML   string
	ESCPOS []byte // populated in Sprint 2 by the escpos builder
}
