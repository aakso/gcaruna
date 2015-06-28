package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aakso/gcaruna"
	"github.com/aakso/gcaruna/output"
)

type (
	OperatingMode uint
	OutputMode    uint
)

const (
	SeriesMode OperatingMode = iota
	LocationMode

	TextOutput OutputMode = iota
	JsonOutput
	InfluxDbOutput

	EnvPrefix = "CARUNA_"
)

type Config struct {
	TimeStart time.Time
	TimeStop  time.Time
	Mode      OperatingMode
	Output    OutputMode
	CarunaUrl string
	Location  string
	Debug     bool
	// InfluxDB output specific
	InfluxDB *output.InfluxDBConfig
	// Internal config parsing stuff
	argmap map[string]interface{}
	*flag.FlagSet
}

// Concept stolen from etcd
func (cfg *Config) mergeEnv() error {
	var err error
	fs := cfg.FlagSet

	setFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})
	fs.VisitAll(func(f *flag.Flag) {
		if setFlags[f.Name] {
			return
		}

		key := EnvPrefix + strings.ToUpper(strings.Replace(f.Name, "-", "_", -1))
		val := os.Getenv(key)
		if val == "" {
			return
		}

		flagErr := fs.Set(f.Name, val)
		if flagErr != nil {
			err = flagErr
		}
	})
	return err
}

func (cfg *Config) Parse(args []string) error {
	var err error

	// Parse flags
	cfg.FlagSet.Parse(args)

	// Merge os envs
	err = cfg.mergeEnv()
	if err != nil {
		return err
	}

	// Parse Operating Mode
	switch *cfg.argmap["mode"].(*string) {
	case "series":
		cfg.Mode = SeriesMode
	case "location":
		cfg.Mode = LocationMode
	default:
		return fmt.Errorf("unknown operating mode")
	}

	// Output mode
	switch *cfg.argmap["output"].(*string) {
	case "text":
		cfg.Output = TextOutput
	case "json":
		cfg.Output = JsonOutput
	case "influxdb":
		cfg.Output = InfluxDbOutput
	default:
		return fmt.Errorf("unknown output mode")
	}

	// Parse timestamps
	cfg.TimeStart, err = time.Parse(time.RFC3339, *cfg.argmap["tstart"].(*string))
	if err != nil {
		return fmt.Errorf("Cannot parse time_start: %s", err)
	}
	cfg.TimeStop, err = time.Parse(time.RFC3339, *cfg.argmap["tstop"].(*string))
	if err != nil {
		return fmt.Errorf("Cannot parse time_stop: %s", err)
	}

	cfg.Debug = *cfg.argmap["debug"].(*bool)
	cfg.CarunaUrl = *cfg.argmap["caruna_url"].(*string)
	cfg.Location = *cfg.argmap["location"].(*string)
	cfg.InfluxDB = &output.InfluxDBConfig{}
	cfg.InfluxDB.Host = *cfg.argmap["influxdb_host"].(*string)
	cfg.InfluxDB.Username = *cfg.argmap["influxdb_username"].(*string)
	cfg.InfluxDB.Password = *cfg.argmap["influxdb_password"].(*string)
	cfg.InfluxDB.Database = *cfg.argmap["influxdb_database"].(*string)
	cfg.InfluxDB.Incremental = *cfg.argmap["influxdb_incremental"].(*bool)
	return nil
}

var (
	config *Config
)

func fatal(errs ...error) {
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
	}
	panic("fatal error")
}

func init() {
}

func NewConfig() *Config {
	cfg := &Config{}

	cfg.FlagSet = flag.NewFlagSet("caruna", flag.ExitOnError)
	fs := cfg.FlagSet
	cfg.argmap = make(map[string]interface{})

	// Defaults
	time_start := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	time_stop := time.Now().Format(time.RFC3339)
	const (
		mode   = "location"
		output = "text"
	)

	cfg.argmap["tstart"] = fs.String("time_start", time_start, "Start time in ISO8601 format")
	cfg.argmap["tstop"] = fs.String("time_stop", time_stop, "Stop time in ISO8601 format")
	cfg.argmap["mode"] = fs.String("mode", mode, "Mode of operation (series, location)")
	cfg.argmap["location"] = fs.String("location", "", "Selected location for the series mode (address or location id)")
	cfg.argmap["output"] = fs.String("output", output, "Output mode (text, json, influxdb)")
	cfg.argmap["caruna_url"] = fs.String("caruna_url", caruna.CarunaAuthStart, "Caruna URL")
	cfg.argmap["debug"] = fs.Bool("debug", false, "true/false")
	cfg.argmap["influxdb_host"] = fs.String("influxdb_host", "", "InfluxDB host:port")
	cfg.argmap["influxdb_username"] = fs.String("influxdb_username", "", "InfluxDB username")
	cfg.argmap["influxdb_password"] = fs.String("influxdb_password", "", "InfluxDB password")
	cfg.argmap["influxdb_database"] = fs.String("influxdb_database", "", "InfluxDB database name")
	cfg.argmap["influxdb_incremental"] = fs.Bool("influxdb_incremental", true, "Whether to check firstval/lastval")
	return cfg
}

func main() {
	username := os.Getenv("CARUNA_USERNAME")
	password := os.Getenv("CARUNA_PASSWORD")

	config = NewConfig()

	err := config.Parse(os.Args[1:])
	if err != nil {
		fatal(err)
	}

	clientOpts := &caruna.ClientOpts{}
	if config.Debug {
		clientOpts.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	client, err := caruna.NewCarunaClient(config.CarunaUrl, username, password, clientOpts)
	if err != nil {
		fatal(err)
	}
	defer client.Logout()

	var res interface{}

	switch config.Mode {
	case LocationMode:
		res, err = client.GetMeteringPoints()
	case SeriesMode:
		res, err = client.GetHourlySeries(config.Location, config.TimeStart, config.TimeStop)
	}
	if err != nil {
		fatal(err)
	}
	switch config.Output {
	case TextOutput:
		output.PrintTextOutput(res)
	case JsonOutput:
		output.PrintJsonOutput(res)
	case InfluxDbOutput:
		vals, ok := res.([]caruna.HourlyEnergyMeasurement)
		if ok {
			influxOutput, err := output.NewInfluxDBOutput(config.InfluxDB)
			if err != nil {
				fatal(err)
			}
			if config.Debug {
				influxOutput.SetLogger(log.New(os.Stderr, "", log.LstdFlags))
			}
			err = influxOutput.WriteData(vals)
			if err != nil {
				fatal(err)
			}
		}
	}
}
