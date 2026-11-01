package value_calculator

import (
	"math"
	"testing"

	"github.com/Scrin/RuuviBridge/parser"
)

// Reference values: independent re-implementation of Picard et al. (2008), CIPM-2007, with the
// d/e term of Z outside the bracket (also in metXact ruuvi-docker/air_density.c, 01.11.2026).
func TestAirDensityCIPM2007(t *testing.T) {
	cases := []struct{ p, temp, rh, rho float64 }{
		{101325, 20, 0, 1.20456},
		{101325, 25, 0, 1.18430},
		{101325, 20, 50, 1.19931},
		{96500, 22, 40, 1.13467},
	}
	for _, c := range cases {
		p, temp, rh := c.p, c.temp, c.rh
		var m parser.Measurement
		m.Pressure, m.Temperature, m.Humidity = &p, &temp, &rh
		CalcExtendedValues(&m)
		if m.AirDensity == nil {
			t.Fatalf("no AirDensity for %+v", c)
		}
		if dev := (*m.AirDensity/c.rho - 1) * 1e6; math.Abs(dev) > 10 {
			t.Errorf("%v Pa %v °C %v %%: %.6f kg/m³, ref %.5f (%+.0f ppm)", p, temp, rh, *m.AirDensity, c.rho, dev)
		}
	}
}
