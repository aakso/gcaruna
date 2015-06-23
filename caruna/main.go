package main

import (
	"fmt"
	"os"
	"time"
	"flag"
	"strings"
	"log"
	"encoding/json"

	"caruna"
)

type(
	OperatingMode uint
	OutputMode uint
)

const (
	SeriesMode OperatingMode = iota
	LocationMode

	TextOutput OutputMode = iota
	JsonOutput
	InfluxDbOutput
)

type Config struct {
	TimeStart time.Time
	TimeStop time.Time
	Mode OperatingMode
	Output OutputMode
	CarunaUrl string
	Debug bool
}

var(
	CmdLineArgs map[string]interface{}
)

func fatal(errs ...error) {
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
	}
	panic("fatal error")
}

func init() {
	initFlags()
}

func newConfiguration() (*Config, error) {
	config := &Config{}
	var err error

	// Parse Operating Mode
	switch(*CmdLineArgs["mode"].(*string)) {
	case "series":
		config.Mode = SeriesMode
	case "location":
		config.Mode = LocationMode
	default:
		return nil, fmt.Errorf("unknown operating mode")
	}

	// Output mode
	switch(*CmdLineArgs["output"].(*string)) {
	case "text":
		config.Output = TextOutput
	case "json":
		config.Output = JsonOutput
	case "influxdb":
		config.Output = InfluxDbOutput
	default:
		return nil, fmt.Errorf("unknown output mode")
	}

	// Parse timestamps
	config.TimeStart, err = time.Parse(time.RFC3339, *CmdLineArgs["tstart"].(*string))
	if err != nil {
		return nil, fmt.Errorf("Cannot parse time_start: %s", err)
	}
	config.TimeStop, err = time.Parse(time.RFC3339, *CmdLineArgs["tstop"].(*string))
	if err != nil {
		return nil, fmt.Errorf("Cannot parse time_stop: %s", err)
	}

	config.CarunaUrl = *CmdLineArgs["caruna_url"].(*string)
	config.Debug = *CmdLineArgs["debug"].(*bool)

	return config, nil
}

func initFlags() {
	// Defaults
	time_start := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	time_stop := time.Now().Format(time.RFC3339)
	const(
		mode = "location"
		output = "text"
	)
	
	CmdLineArgs = make(map[string]interface{})
	CmdLineArgs["tstart"] = flag.String("time_start", time_start, "Start time in ISO8601 format")
	CmdLineArgs["tstop"] = flag.String("time_stop", time_stop, "Stop time in ISO8601 format")
	CmdLineArgs["mode"] = flag.String("mode", mode, "Mode of operation (series, location)")
	CmdLineArgs["output"] = flag.String("output", output, "Output mode (text, json, influxdb)")
	CmdLineArgs["caruna_url"] = flag.String("caruna_url", caruna.CarunaAuthStart, "Caruna URL")
	CmdLineArgs["debug"] = flag.Bool("debug", false, "true/false")

}

func PrintTextMeteringPoints(mps []caruna.MeteringPoint) {
	for i, e := range mps {
		if i != 0 {
			fmt.Println()
		}
		fmt.Printf("Metering point %d:\n", i)
		fmt.Printf("%-20s%s\n", "Location:", strings.Join(e.Location, " "))
		fmt.Printf("%-20s%s\n", "Id:", e.MeteringPointNumber)
		fmt.Printf("%-20s%s\n", "Type:", e.MeteringPointType)
		fmt.Printf("%-20s%t\n", "Hourly measured:", e.HourlyMeasured)
		fmt.Printf("%-20s%s\n", "Contract begin:", e.Created)

	}
}

func PrintTextOutput(output interface{}) {
	switch v := output.(type) {
	case []caruna.MeteringPoint:
		PrintTextMeteringPoints(v)
	}
}

func PrintJsonOutput(output interface{}) {
	switch v := output.(type) {
	case []caruna.MeteringPoint:
		bs, err := json.MarshalIndent(v, "", "    ")
		if err == nil {
			fmt.Println(string(bs))
		}
	}
}

func main() {
	username := os.Getenv("CARUNA_USERNAME")
	password := os.Getenv("CARUNA_PASSWORD")

	flag.Parse()
	config, err := newConfiguration()
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

	var output interface{}

	switch config.Mode {
	case LocationMode:
		output, err = client.GetMeteringPoints()
	}
	if err != nil {
		fatal(err)
	}
	switch config.Output {
	case TextOutput:
		PrintTextOutput(output)
	case JsonOutput:
		PrintJsonOutput(output)
	}
}
