package value_calculator

import (
	"math"

	"github.com/Scrin/RuuviBridge/parser"
)

// CIPM-2007 / BIPM saturation vapour pressure coefficients (psv in Pa, T in K).
const (
	cipmA = 1.2378847e-5
	cipmB = -1.9121316e-2
	cipmC = 33.93711047
	cipmD = -6.3431645e3
)

// cipmPsv returns the saturation vapour pressure of water (Pa) at temperature
// tK (Kelvin) per the CIPM-2007 / BIPM formula psv = exp(A·T² + B·T + C + D/T).
// It is the single source of truth for saturation vapour pressure, shared by the
// EquilibriumVaporPressure field, the dew point and the moist-air density, so all
// humidity-derived values stay mutually consistent.
func cipmPsv(tK float64) float64 {
	return math.Exp(cipmA*tK*tK + cipmB*tK + cipmC + cipmD/tK)
}

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
		// CIPM-2007 / BIPM saturation vapour pressure (Pa), the same formula used
		// by the moist-air density below (was previously a Magnus/Bolton fit).
		m.EquilibriumVaporPressure = f64(cipmPsv(*m.Temperature + 273.15))
	}
	if m.Temperature != nil && m.Humidity != nil {
		m.AbsoluteHumidity = f64((*m.EquilibriumVaporPressure) * (*m.Humidity) * 0.021674 / (273.15 + (*m.Temperature)))
	}
	if m.EquilibriumVaporPressure != nil && m.Temperature != nil && m.Humidity != nil && *m.Humidity > 0 {
		// Dew point: invert the same CIPM saturation curve numerically (Newton),
		// so Td is consistent with EquilibriumVaporPressure instead of a Magnus fit.
		e := (*m.Humidity / 100) * (*m.EquilibriumVaporPressure) // actual vapour pressure (Pa)
		td := *m.Temperature                                     // start from air temperature (°C)
		for i := 0; i < 30; i++ {
			tK := td + 273.15
			psv := cipmPsv(tK)
			dpsv := psv * (2*cipmA*tK + cipmB - cipmD/(tK*tK)) // d(psv)/dT
			if dpsv == 0 {
				break
			}
			step := (psv - e) / dpsv
			td -= step
			if math.Abs(step) < 1e-7 {
				break
			}
		}
		m.DewPoint = f64(td)
	}
	if m.Temperature != nil && m.Humidity != nil && m.Pressure != nil {
		// CIPM-2007: Picard, Davis, Gläser, Fuji — revised formula for the density of moist air
		// Pressure input is in Pa; temperature in °C; humidity in %; CO2 in ppm (optional)
		const R = 8.31447215      // gas constant J/(mol·K)
		const M_v = 18.0152817e-3 // molar mass of water vapour kg/mol
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

		// Saturation vapour pressure (CIPM/BIPM formula) — shared with EVP and dew point
		pSV := cipmPsv(T)

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
		m.KinematicViscosity = f64(*m.AirViscosity / *m.AirDensity * 1e6) // mm²/s
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
