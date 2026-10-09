package ar

import "testing"

// TestAfipRejectionIsPermanent covers the retryable-vs-permanent classification,
// including the non-correlative sequence-number rejection that a same-series race
// can trigger — it MUST be retryable so a retry re-sequences past it rather than
// permanently abandoning the invoice.
func TestAfipRejectionIsPermanent(t *testing.T) {
	cases := []struct {
		name      string
		resultado string
		errs      []afipMessage
		obs       []afipMessage
		want      bool
	}{
		{
			name:      "definitive rejection is permanent",
			resultado: "R",
			errs:      []afipMessage{{Code: 10048, Msg: "El campo DocNro es invalido"}},
			want:      true,
		},
		{
			name:      "non-correlative sequence rejection is retryable",
			resultado: "R",
			errs:      []afipMessage{{Code: 10016, Msg: "El campo CbteDesde ingresado no es correlativo al ultimo comprobante autorizado"}},
			want:      false,
		},
		{
			name:      "infra transient rejection is retryable",
			resultado: "R",
			errs:      []afipMessage{{Code: 500, Msg: "El servicio no se encuentra disponible, intente nuevamente"}},
			want:      false,
		},
		{
			name:      "non-R result is always retryable",
			resultado: "",
			errs:      []afipMessage{{Code: 600, Msg: "ambiguous"}},
			want:      false,
		},
		{
			name:      "correlativo can arrive as an observation, still retryable",
			resultado: "R",
			obs:       []afipMessage{{Code: 10016, Msg: "no es correlativo"}},
			want:      false,
		},
		{
			name:      "code 10016 is retryable even without correlativo",
			resultado: "R",
			errs:      []afipMessage{{Code: 10016, Msg: "El numero de comprobante no se corresponde con el proximo a autorizar"}},
			want:      false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := afipRejectionIsPermanent(c.resultado, c.errs, c.obs); got != c.want {
				t.Errorf("afipRejectionIsPermanent(%q) = %v, want %v", c.resultado, got, c.want)
			}
		})
	}
}
