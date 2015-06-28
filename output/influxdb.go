package output

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/aakso/gcaruna"
	influxdb "github.com/aakso/gcaruna/influxdb08"
)

type InfluxDBConfig struct {
	Host        string
	Username    string
	Password    string
	Database    string
	Incremental bool
}

const (
	SeriesName = "kwh.%s"
	FieldName  = "value"
)

type InfluxDBOutput struct {
	logger *log.Logger
	Client *influxdb.Client
	Config *InfluxDBConfig
}

func (self *InfluxDBOutput) SetLogger(logger *log.Logger) {
	self.logger = logger
	self.logger.SetPrefix("[InfluxDBOutput] ")
}

func (self *InfluxDBOutput) WriteData(hms []caruna.HourlyEnergyMeasurement) error {
	var err error

	self.logger.Println("Start WriteData")
	// Timebounds for incremental runs
	limitRanges := make(map[string][]time.Time)

	// Data in influxdb series
	seriesToDb := make(map[string]*influxdb.Series)

	for _, e := range hms {
		seriesName := getSeriesName(e.MeteringPointLocation)
		if _, keyexists := seriesToDb[seriesName]; keyexists == false {
			seriesToDb[seriesName] = &influxdb.Series{
				Name:    seriesName,
				Columns: []string{"time", "value"},
			}
		}
		limitRange, limitRangeFound := limitRanges[seriesName]
		var limitStart, limitStop time.Time
		if limitRangeFound {
			limitStart = limitRange[0]
			limitStop = limitRange[1]
		} else {
			// By default the time range is now to now
			limitStart = time.Now()
			limitStop = time.Now()
		}

		// Query for ranges if incremental run is requested
		if self.Config.Incremental && !limitRangeFound {
			// Check that the series exists
			q := fmt.Sprintf("LIST SERIES /^%s$/", seriesName)
			res, err := self.Client.Query(q)
			if err != nil {
				return err
			}
			seriesExists := len(res[0].Points) > 0

			if seriesExists {
				var val int64
				q = fmt.Sprintf("SELECT * FROM %s ORDER ASC LIMIT 1", seriesName)
				res, err = self.Client.Query(q, influxdb.Second)
				if err != nil {
					return err
				}
				cid, err := getInfluxDBSeriesColumnId(res[0], "time")
				if err != nil {
					return err
				}
				val = int64(res[0].Points[0][cid].(float64))
				limitStart = time.Unix(val, 0)

				q = fmt.Sprintf("SELECT * FROM %s ORDER DESC LIMIT 1", seriesName)
				res, err = self.Client.Query(q, influxdb.Second)
				if err != nil {
					return err
				}
				cid, err = getInfluxDBSeriesColumnId(res[0], "time")
				if err != nil {
					return err
				}
				val = int64(res[0].Points[0][cid].(float64))
				limitStop = time.Unix(val, 0)

				// Cache ranges
				limitRanges[seriesName] = []time.Time{limitStart, limitStop}
				self.logger.Printf("Series %s: excluding range %s - %s", seriesName, limitStart.String(), limitStop.String())
			}
		} // query time ranges

		// Skip measurements that are in the limit range (incremental mode)
		mts := e.Timestamp
		if self.Config.Incremental &&
			(mts.Equal(limitStart) || mts.After(limitStart)) &&
			(mts.Equal(limitStop) || mts.Before(limitStop)) {

			continue
		}

		// Make influxdb data point array
		points := []interface{}{
			e.Timestamp.Unix(),
			e.Value,
		}
		seriesToDb[seriesName].Points = append(seriesToDb[seriesName].Points, points)
	} // range hms

	count := 0
	for _, v := range seriesToDb {
		err = self.Client.WriteSeriesWithTimePrecision([]*influxdb.Series{v}, influxdb.Second)
		if err != nil {
			return err
		}
		count += len(v.Points)
	}

	self.logger.Printf("Wrote %d data points to DB\n", count)
	return nil
}

func NewInfluxDBOutput(config *InfluxDBConfig) (*InfluxDBOutput, error) {
	var err error
	clientCfg := &influxdb.ClientConfig{
		Host:     config.Host,
		Username: config.Username,
		Password: config.Password,
		Database: config.Database,
	}
	client, err := influxdb.NewClient(clientCfg)
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

func getInfluxDBSeriesColumnId(series *influxdb.Series, column string) (int, error) {
	idx := -1
	for i, v := range series.Columns {
		if v == column {
			idx = i
		}
	}
	if idx == -1 {
		return -1, fmt.Errorf("Column %s not found", column)
	}
	return idx, nil
}

func getSeriesName(loc []string) string {
	ret := strings.Join(loc, "_")
	ret = strings.Replace(ret, " ", "_", -1)
	return fmt.Sprintf(SeriesName, ret)
}
