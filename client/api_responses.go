package caruna

type CustomerInfo struct {
	Username string
	Created  string
	Modified string
	Deleted  string
	Email    string
	Locale   string
	Active   bool
}

type MeteringEntities struct {
	Entities []MeteringEntity
}

type MeteringEntity struct {
	MeteringPoint *RawMeteringPoint
}

type RawMeteringPoint struct {
	Created             string
	Modified            string
	Deleted             string
	MeteringPointNumber string
	MeteringPointType   string
	HourlyMeasured      bool
	Address             *MeteringPointAddress
	AddressStr          string
}

type MeteringPointAddress struct {
	Street  string
	ZipCode string
	City    string
}

type RawMeasurement struct {
	HourlyMeasured bool
	UTCOffset      float64
	Timestamp      string
	Values         *MeasurementValues
}

type MeasurementValues struct {
	EnergyConsumption *EnergyConsumptionValue `json:"EL_ENERGY_CONSUMPTION#0"`
}

type EnergyConsumptionValue struct {
	Value  float64 `json:"valueAsFloat"`
	Status string  `json:"statusAsSeriesStatus"`
}
