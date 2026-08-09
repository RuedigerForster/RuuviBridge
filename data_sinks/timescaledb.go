package data_sinks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/RuuviBridge/common/limiter"
	"github.com/Scrin/RuuviBridge/config"
	"github.com/Scrin/RuuviBridge/parser"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// TimescaleDB writes measurements into a PostgreSQL/TimescaleDB long-format
// hypertable (sensor_readings) plus a sensors dimension table. Mirrors the
// InfluxDB3 sink's lifecycle: a buffered channel, a per-mac minimum_interval
// limiter, and one goroutine per accepted measurement.
func TimescaleDB(conf config.TimescaleDBPublisher) chan<- parser.Measurement {
	host := conf.Host
	if host == "" {
		host = "timescaledb"
	}
	port := conf.Port
	if port == 0 {
		port = 5432
	}
	database := conf.Database
	if database == "" {
		database = "metxact"
	}
	user := conf.User
	if user == "" {
		user = "metxact"
	}
	sslmode := conf.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	readingsTable := conf.Table
	if readingsTable == "" {
		readingsTable = "sensor_readings"
	}
	sensorsTable := conf.SensorsTable
	if sensorsTable == "" {
		sensorsTable = "sensors"
	}

	// Keyword/value DSN — avoids URL-encoding the password.
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s application_name=ruuvibridge",
		host, port, user, conf.Password, database, sslmode,
	)

	log.Info().
		Str("target", fmt.Sprintf("%s:%d/%s", host, port, database)).
		Str("readings_table", readingsTable).
		Dur("minimum_interval", conf.MinimumInterval).
		Msg("Starting TimescaleDB sink")

	poolConf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse TimescaleDB connection config")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create TimescaleDB connection pool")
	}

	sensorsUpsert := fmt.Sprintf(
		"INSERT INTO %s (sensor_id, name, data_format, last_seen) VALUES ($1,$2,$3,$4) "+
			"ON CONFLICT (sensor_id) DO UPDATE SET "+
			"name = COALESCE(EXCLUDED.name, %s.name), "+
			"data_format = EXCLUDED.data_format, last_seen = EXCLUDED.last_seen",
		sensorsTable, sensorsTable,
	)
	readingInsert := fmt.Sprintf(
		"INSERT INTO %s (time, sensor_id, metric, value) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING",
		readingsTable,
	)

	limiter := limiter.New(conf.MinimumInterval)
	measurements := make(chan parser.Measurement, 1024)
	go func() {
		for measurement := range measurements {
			if !limiter.Check(measurement) {
				log.Trace().Str("mac", measurement.Mac).Msg("Skipping TimescaleDB publish due to interval limit")
				continue
			}
			go func(measurement parser.Measurement) {
				if pool == nil {
					return
				}
				ts := time.Now().UTC()
				sensorID := strings.ToUpper(strings.ReplaceAll(measurement.Mac, ":", ""))

				var rows []metricRow
				addFloat := func(name string, v *float64) {
					if v != nil {
						rows = append(rows, metricRow{name, *v})
					}
				}
				addInt := func(name string, v *int64) {
					if v != nil {
						rows = append(rows, metricRow{name, float64(*v)})
					}
				}
				addBool := func(name string, v *bool) {
					if v != nil {
						val := 0.0
						if *v {
							val = 1.0
						}
						rows = append(rows, metricRow{name, val})
					}
				}

				addFloat("temperature", measurement.Temperature)
				addFloat("humidity", measurement.Humidity)
				addFloat("pressure", measurement.Pressure)
				addFloat("accelerationX", measurement.AccelerationX)
				addFloat("accelerationY", measurement.AccelerationY)
				addFloat("accelerationZ", measurement.AccelerationZ)
				addFloat("batteryVoltage", measurement.BatteryVoltage)
				addInt("txPower", measurement.TxPower)
				addInt("rssi", measurement.Rssi)
				addInt("movementCounter", measurement.MovementCounter)
				addInt("measurementSequenceNumber", measurement.MeasurementSequenceNumber)
				addFloat("accelerationTotal", measurement.AccelerationTotal)
				addFloat("absoluteHumidity", measurement.AbsoluteHumidity)
				addFloat("dewPoint", measurement.DewPoint)
				addFloat("equilibriumVaporPressure", measurement.EquilibriumVaporPressure)
				addFloat("airDensity", measurement.AirDensity)
				addFloat("airViscosity", measurement.AirViscosity)
				addFloat("kinematicViscosity", measurement.KinematicViscosity)
				addFloat("accelerationAngleFromX", measurement.AccelerationAngleFromX)
				addFloat("accelerationAngleFromY", measurement.AccelerationAngleFromY)
				addFloat("accelerationAngleFromZ", measurement.AccelerationAngleFromZ)
				// RuuviAir (format E1) air-quality fields
				addFloat("pm1p0", measurement.Pm1p0)
				addFloat("pm2p5", measurement.Pm2p5)
				addFloat("pm4p0", measurement.Pm4p0)
				addFloat("pm10p0", measurement.Pm10p0)
				addFloat("co2", measurement.CO2)
				addFloat("voc", measurement.VOC)
				addFloat("nox", measurement.NOX)
				addFloat("illuminance", measurement.Illuminance)
				addFloat("soundInstant", measurement.SoundInstant)
				addFloat("soundAverage", measurement.SoundAverage)
				addFloat("soundPeak", measurement.SoundPeak)
				addFloat("airQualityIndex", measurement.AirQualityIndex)
				// Diagnostics (stored as 0/1)
				addBool("calibrationInProgress", measurement.CalibrationInProgress)
				addBool("buttonPressedOnBoot", measurement.ButtonPressedOnBoot)
				addBool("rtcOnBoot", measurement.RtcOnBoot)

				if len(rows) == 0 {
					return
				}

				var dataFormat *int64
				if measurement.DataFormat != 0 {
					df := measurement.DataFormat
					dataFormat = &df
				}

				batch := &pgx.Batch{}
				batch.Queue(sensorsUpsert, sensorID, measurement.Name, dataFormat, ts)
				for _, r := range rows {
					batch.Queue(readingInsert, ts, sensorID, r.metric, r.value)
				}

				br := pool.SendBatch(context.Background(), batch)
				defer br.Close()
				for range batch.QueuedQueries {
					if _, err := br.Exec(); err != nil {
						log.Error().Err(err).Str("mac", measurement.Mac).Msg("Failed to send data to TimescaleDB")
						return
					}
				}
			}(measurement)
		}
		if pool != nil {
			pool.Close()
		}
	}()
	return measurements
}

type metricRow struct {
	metric string
	value  float64
}
