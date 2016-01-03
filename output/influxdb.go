package output

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/aakso/gcaruna"
	influxdb "github.com/influxdb/influxdb/client/v2"
)

type InfluxDBConfig struct {
	URL         string
	Username    string
	Password    string
	Database    string
	Incremental bool
}

const (
	SeriesName = "gcaruna"
	TagName = "meteringpoint"
	FieldName  = "value"
)

type InfluxDBOutput struct {
	logger *log.Logger
	Client influxdb.Client
	Config *InfluxDBConfig
}

func (self *InfluxDBOutput) SetLogger(logger *log.Logger) {
	self.logger = logger
	self.logger.SetPrefix("[InfluxDBOutput] ")
}

func (self *InfluxDBOutput) WriteData(hms []caruna.HourlyEnergyMeasurement) error {
	self.logger.Println("Start WriteData")
	// Timebounds for incremental runs
	limitRanges := make(map[string][]time.Time)

	// Batch for points
	bp, _ := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database: self.Config.Database,
		Precision: "s",
	})

	for _, e := range hms {
		meteringPointName := getMeteringPointName(e.MeteringPointLocation)

		limitRange, limitRangeFound := limitRanges[meteringPointName]
		var limitStart, limitStop time.Time
		if limitRangeFound && limitRange != nil {
			limitStart = limitRange[0]
			limitStop = limitRange[1]
		} else {
			// By default the time range is now to now
			limitStart = time.Now()
			limitStop = time.Now()
		}

		// Query for ranges if incremental run is requested
		if self.Config.Incremental && !limitRangeFound {
			q := fmt.Sprintf(`SELECT * FROM "%s" WHERE %s='%s' ORDER BY time ASC LIMIT 1`, SeriesName, TagName, meteringPointName)
			res, err := self.query(q)
			if err != nil {
				return err
			}
			if len(res[0].Series) > 0 {
				val, _ := time.Parse(time.RFC3339, res[0].Series[0].Values[0][0].(string))
				limitStart = val

				q = fmt.Sprintf(`SELECT * FROM "%s" WHERE %s='%s' ORDER BY time DESC LIMIT 1`, SeriesName, TagName, meteringPointName)
				res, err = self.query(q)
				if err != nil {
					return err
				}
				val, _ = time.Parse(time.RFC3339, res[0].Series[0].Values[0][0].(string))
				limitStop = val

				// Cache ranges
				limitRanges[meteringPointName] = []time.Time{limitStart, limitStop}
				self.logger.Printf("Meteringpoint %s: excluding range %s - %s", meteringPointName, limitStart.String(), limitStop.String())
			} else {
				self.logger.Printf("No existing time range for meteringpoint: %s", meteringPointName)
				limitRanges[meteringPointName] = nil
			}
		} // query time ranges

		// Skip measurements that are in the limit range (incremental mode)
		mts := e.Timestamp
		if self.Config.Incremental &&
			(mts.Equal(limitStart) || mts.After(limitStart)) &&
			(mts.Equal(limitStop) || mts.Before(limitStop)) {

			continue
		}

		// Make influxdb point
		tags := map[string]string{
			"meteringpoint": meteringPointName,
		}
		fields := map[string]interface{}{
			FieldName: e.Value,
		}
		pt, err := influxdb.NewPoint(
			SeriesName,
			tags,
			fields,
			e.Timestamp,
		)
		if err != nil {
			return err
		}
		bp.AddPoint(pt)
	} // range hms


	if err := self.Client.Write(bp); err != nil {
		return err
	}

	self.logger.Printf("Wrote %d data points to DB\n", len(bp.Points()))
	return nil
}

func (self *InfluxDBOutput) query(cmd string) (res []influxdb.Result, err error) {
    q := influxdb.Query{
        Command:  cmd,
        Database: self.Config.Database,
    }
    if response, err := self.Client.Query(q); err == nil {
        if response.Error() != nil {
            return res, response.Error()
        }
        res = response.Results
    }
    return res, nil
}

func NewInfluxDBOutput(config *InfluxDBConfig) (*InfluxDBOutput, error) {
	var err error
	clientCfg := influxdb.HTTPConfig{
		Addr:     config.URL,
		Username: config.Username,
		Password: config.Password,
	}
	client, err := influxdb.NewHTTPClient(clientCfg)
	if err != nil {
		return nil, err
	}

	ret := &InfluxDBOutput{
		Client: client,
		Config: config,
	}
	ret.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	return ret, nil
}

func getMeteringPointName(loc []string) string {
	ret := strings.Join(loc, "_")
	ret = strings.Replace(ret, " ", "_", -1)
	return ret
}
