package value_calculator

import (
	"math"

	"github.com/Scrin/RuuviBridge/parser"
)

func CalcExtendedValues(m *parser.Measurement) {
	// from https://github.com/Scrin/RuuviCollector/blob/master/src/main/java/fi/tkgwf/ruuvi/utils/MeasurementValueCalculator.java
	f64 := func(value float64) *float64 { return &value }
	if m.AccelerationX != nil && m.AccelerationY != nil && m.AccelerationZ != nil {
		m.AccelerationTotal = f64(math.Sqrt((*m.AccelerationX)*(*m.AccelerationX) + (*m.AccelerationY)*(*m.AccelerationY) + (*m.AccelerationZ)*(*m.AccelerationZ)))
	}
	if m.AccelerationX != nil && m.AccelerationTotal != nil && *m.AccelerationTotal != 0 {
		m.AccelerationAngleFromX = f64(math.Acos((*m.AccelerationX)/(*m.AccelerationTotal)) * (180 / math.Pi))
	}
	if m.AccelerationY != nil && m.AccelerationTotal != nil && *m.AccelerationTotal != 0 {
		m.AccelerationAngleFromY = f64(math.Acos((*m.AccelerationY)/(*m.AccelerationTotal)) * (180 / math.Pi))
	}
	if m.AccelerationZ != nil && m.AccelerationTotal != nil && *m.AccelerationTotal != 0 {
		m.AccelerationAngleFromZ = f64(math.Acos((*m.AccelerationZ)/(*m.AccelerationTotal)) * (180 / math.Pi))
	}
	if m.Temperature != nil {
		m.EquilibriumVaporPressure = f64(611.2 * math.Exp(17.67*(*m.Temperature)/(243.5+(*m.Temperature))))
	}
	if m.Temperature != nil && m.Humidity != nil {
		m.AbsoluteHumidity = f64((*m.EquilibriumVaporPressure) * (*m.Humidity) * 0.021674 / (273.15 + (*m.Temperature)))
	}
	if m.EquilibriumVaporPressure != nil && m.Humidity != nil && *m.Humidity != 0 {
		v := math.Log((*m.Humidity) / 100 * (*m.EquilibriumVaporPressure) / 611.2)
		m.DewPoint = f64(-243.5 * v / (v - 17.67))
	}
	if m.Temperature != nil && m.Humidity != nil && m.Pressure != nil {
		// CIPM-2007: Picard, Davis, Gläser, Fuji — revised formula for the density of moist air
		// Pressure input is in Pa; temperature in °C; humidity in %; CO2 in ppm (optional)
		const R = 8.31447215      // gas constant J/(mol·K)
		const M_v = 18.0152817e-3 // molar mass of water vapour kg/mol
		const cA = 1.2378847e-5
		const cB = -1.9121316e-2
		const cC = 33.93711047
		const cD = -6.3431645e3
		const cAlpha = 1.00062
		const cBeta = 3.14e-8
		const cGamma = 5.6e-7
		const ca0 = 1.58123e-6
		const ca1 = -2.9331e-8
		const ca2 = 1.1043e-10
		const cb0 = 5.707e-6
		const cb1 = -2.051e-8
		const cc0 = 1.9898e-4
		const cc1 = -2.376e-6
		const cd = 1.83e-11
		const ce = -0.765e-8

		M_a := 28.96546e-3 // molar mass of dry air kg/mol
		if m.CO2 != nil {
			xCO2 := *m.CO2 / 1e6 // ppm to mole fraction
			if xCO2 >= 0.0004 {
				M_a += 12.011e-3 * (xCO2 - 0.0004)
			}
		}

		t := *m.Temperature
		P := *m.Pressure // Pa
		T := t + 273.15  // K

		// Saturation vapour pressure (BIPM formula)
		pSV := math.Exp(cA*T*T + cB*T + cC + cD/T)

		// Enhancement factor
		f := cAlpha + cBeta*P + cGamma*t*t

		// Mole fraction of water vapour
		xH2O := (*m.Humidity / 100) * f * pSV / P

		// Compressibility factor
		Z := 1 - (P/T)*(ca0+ca1*t+ca2*t*t+
			(cb0+cb1*t)*xH2O+
			(cc0+cc1*t)*xH2O*xH2O+
			P*P/T/T*(cd+ce*xH2O*xH2O))

		m.AirDensity = f64(P * M_a / Z / R / T * (1 - xH2O*(1-M_v/M_a)))
	}
	if m.Temperature != nil {
		// Sutherland's formula for dynamic viscosity of air in Pa·s
		// Reference: μ_ref=18.27 μPa·s at T_ref=291.15 K, Sutherland constant S=120 K
		const muRef = 18.27e-6
		const tRef = 291.15
		const s = 120.0
		T := *m.Temperature + 273.15
		m.AirViscosity = f64(muRef * math.Pow(T/tRef, 1.5) * (tRef + s) / (T + s))
	}
	if m.AirViscosity != nil && m.AirDensity != nil && *m.AirDensity != 0 {
		m.KinematicViscosity = f64(*m.AirViscosity / *m.AirDensity)
	}
	if m.Pm2p5 != nil && m.CO2 != nil {
		const aqiMax = 100.0
		const pm25Min = 0.0
		const pm25Max = 60.0
		const co2Min = 420.0
		const co2Max = 2300.0
		const pm25Scale = aqiMax / (pm25Max - pm25Min)
		const co2Scale = aqiMax / (co2Max - co2Min)

		pm25 := math.Max(pm25Min, math.Min(pm25Max, *m.Pm2p5))
		co2 := math.Max(co2Min, math.Min(co2Max, *m.CO2))

		dx := (pm25 - pm25Min) * pm25Scale
		dy := (co2 - co2Min) * co2Scale

		r := math.Hypot(dx, dy)
		aqi := math.Max(0, math.Min(aqiMax, aqiMax-r))
		m.AirQualityIndex = f64(aqi)
	}
}
