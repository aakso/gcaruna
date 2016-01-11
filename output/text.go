package output

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aakso/gcaruna/client"
)

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

func PrintTextHourlyMeasurements(hms []caruna.HourlyEnergyMeasurement) {
	w := tabwriter.NewWriter(os.Stdout, 30, 8, 0, '\t', 0)
	header := []string{"Ts", "Loc", "KWh"}
	fmt.Fprintln(w, strings.Join(header, "\t"))
	sum := 0.0
	for _, e := range hms {
		line := []string{
			e.Timestamp.Format(time.RFC3339),
			strings.Join(e.MeteringPointLocation, " "),
			fmt.Sprintf("%f", e.Value),
		}
		fmt.Fprintln(w, strings.Join(line, "\t"))
		sum += e.Value
	}
	fmt.Fprintf(w, "\t\tSum: %f\n", sum)
	w.Flush()
}

func PrintTextOutput(output interface{}) {
	switch v := output.(type) {
	case []caruna.MeteringPoint:
		PrintTextMeteringPoints(v)
	case []caruna.HourlyEnergyMeasurement:
		PrintTextHourlyMeasurements(v)
	}
}
