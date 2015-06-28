package caruna

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/aakso/gcaruna/parser"
)

const (
	CarunaBase      = "https://energiaseuranta.caruna.fi"
	CarunaAuthStart = CarunaBase + "/mobile"
	CarunaSSOLogout = "/portal/logout" // Relative to SSO portal

	// Caruna login form components
	CarunaLoginFormId        = "usernameLogin4"
	CarunaLoginFieldUsername = "ttqusername"
	CarunaLoginFieldPassword = "password"

	// Caruna API paths
	CarunaApiUriCurrentUser    = CarunaBase + "/api/users?current"
	CarunaApiUriMeteringPoints = CarunaBase + "/api/customers/%s/meteringPointInformationWrappers" // params: Username
	CarunaApiUriSeries         = CarunaBase + "/api/meteringPoints/ELECTRICITY/%s/series"          // params: metering point id
	CarunaApiLogout            = CarunaBase + "/api/logout"

	// Caruna API Series query parameters
	CarunaApiSeriesQueryParamTimeStart       = "startDate"
	CarunaApiSeriesQueryParamTimeStop        = "endDate"
	CarunaApiSeriesQueryParamProduct         = "products"
	CarunaApiSeriesQueryParamResolution      = "resolution"
	CarunaApiSeriesQueryParamResolutionValue = "MONTHS_AS_HOURS"
	CarunaApiSeriesQueryParamProductValue    = "EL_ENERGY_CONSUMPTION"

	// Caruna API uses ISO8601 timestamps but doesn't allow the Z sign
	CarunaTimeLayout = "2006-01-02T15:04:05-0700"
)

type PageResponse struct {
	Body         *bytes.Reader
	OrigResponse *http.Response
	Data         []byte
}

type HourlyEnergyMeasurement struct {
	MeteringPointId       string
	MeteringPointLocation []string
	Timestamp             time.Time
	Value                 float64
}

type MeteringPoint struct {
	Created             string
	Modified            string
	Deleted             string
	MeteringPointNumber string
	MeteringPointType   string
	HourlyMeasured      bool
	Location            []string
}

type ClientOpts struct {
	Logger *log.Logger
}

type CarunaClient struct {
	AuthUrl      string
	Client       *http.Client
	CustomerInfo *CustomerInfo
	Logger       *log.Logger

	refUrl *url.URL
}

func (self *CarunaClient) PostPage(urlStr string, vals *url.Values) (*PageResponse, error) {
	self.Logger.Println("Start POST query:", urlStr)
	resp, err := self.Client.PostForm(urlStr, *vals)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	presp, err := self.processResponse(resp)
	if err != nil {
		return nil, err
	}

	return presp, nil
}

func (self *CarunaClient) GetPage(urlStr string) (*PageResponse, error) {
	self.Logger.Println("Start GET query:", urlStr)
	resp, err := self.Client.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	presp, err := self.processResponse(resp)
	if err != nil {
		return nil, err
	}

	return presp, nil
}

func (self *CarunaClient) processResponse(resp *http.Response) (*PageResponse, error) {
	presp := &PageResponse{}
	var err error

	presp.OrigResponse = resp

	if resp.StatusCode != http.StatusOK {
		return presp, fmt.Errorf("Non-ok http status: %+v", resp)
	}

	presp.Data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return presp, err
	}

	presp.Body = bytes.NewReader(presp.Data)

	redirect_url, err := parser.FindMetaRefresh(presp.Body)
	_, err = presp.Body.Seek(0, 0)
	if redirect_url != "" {
		self.Logger.Println("Meta refresh redirect..")
		newUrl, _ := resp.Request.URL.Parse(redirect_url)
		presp, err = self.GetPage(newUrl.String())
	}
	if err != nil {
		return presp, err
	}

	return presp, nil
}

func (self *CarunaClient) GetCustomerInfo() (*CustomerInfo, error) {
	url, err := self.refUrl.Parse(CarunaApiUriCurrentUser)
	if err != nil {
		return nil, err
	}

	resp, err := self.GetPage(url.String())
	if err != nil {
		return nil, err
	}

	ret := &CustomerInfo{}
	err = json.Unmarshal(resp.Data, ret)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse Caruna Customer Info: %s", err)
	}
	return ret, nil
}

func (self *CarunaClient) GetMeteringPoints() ([]MeteringPoint, error) {
	// Meteringpoint url requires customer id
	url := fmt.Sprintf(CarunaApiUriMeteringPoints, self.CustomerInfo.Username)
	resp, err := self.GetPage(url)
	if err != nil {
		return nil, err
	}

	entities := &MeteringEntities{}
	err = json.Unmarshal(resp.Data, entities)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse Caruna Metering Points: %s", err)
	}

	ret := make([]MeteringPoint, len(entities.Entities))
	for i, v := range entities.Entities {
		mp := MeteringPoint{
			Created:             v.MeteringPoint.Created,
			Modified:            v.MeteringPoint.Modified,
			Deleted:             v.MeteringPoint.Deleted,
			MeteringPointNumber: v.MeteringPoint.MeteringPointNumber,
			MeteringPointType:   v.MeteringPoint.MeteringPointType,
			HourlyMeasured:      v.MeteringPoint.HourlyMeasured,
		}
		mp.Location = []string{
			v.MeteringPoint.Address.Street,
			v.MeteringPoint.Address.ZipCode,
			v.MeteringPoint.Address.City,
		}
		ret[i] = mp
	}
	return ret, nil
}

func (self *CarunaClient) GetHourlySeries(meteringPointStr string, timeStart, timeStop time.Time) ([]HourlyEnergyMeasurement, error) {
	if !timeStart.Before(timeStop) {
		return nil, fmt.Errorf("timeStart is after timeStop")
	}

	rawMeasurements := make([]RawMeasurement, 0)
	ret := make([]HourlyEnergyMeasurement, 0)

	meteringPoints, err := self.GetMeteringPoints()
	if err != nil {
		return nil, fmt.Errorf("Cannot get metering points: %s", err)
	}

	// Loop over all meteringpoints
	for _, e := range meteringPoints {
		meteringPointId := e.MeteringPointNumber
		meteringPointLocation := e.Location

		// If user has specified a meteringpoint filter, evaluate and skip all that don't match
		if meteringPointStr != "" {
			match := e.MeteringPointNumber == meteringPointStr ||
				strings.Contains(strings.Join(e.Location, " "), meteringPointStr)

			if !match {
				self.Logger.Println("Skipping meteringpoint:", meteringPointId)
				continue

			}
		}

		// Construct url and parameters
		reqUrl, err := url.Parse(fmt.Sprintf(CarunaApiUriSeries, meteringPointId))
		if err != nil {
			return nil, err
		}
		params := &url.Values{}

		params.Set(CarunaApiSeriesQueryParamProduct, CarunaApiSeriesQueryParamProductValue)
		params.Set(CarunaApiSeriesQueryParamResolution, CarunaApiSeriesQueryParamResolutionValue)
		params.Set(CarunaApiSeriesQueryParamTimeStart, timeStart.Format(CarunaTimeLayout))
		params.Set(CarunaApiSeriesQueryParamTimeStop, timeStop.Format(CarunaTimeLayout))

		reqUrl.RawQuery = params.Encode()

		resp, err := self.GetPage(reqUrl.String())
		if err != nil {
			return nil, err
		}

		// Parse response
		err = json.Unmarshal(resp.Data, &rawMeasurements)
		if err != nil {
			return nil, err
		}

		// Make response
		for _, v := range rawMeasurements {
			// Skip missing values
			if !v.HourlyMeasured {
				continue
			}

			ts, err := time.Parse(CarunaTimeLayout, v.Timestamp)
			if err != nil {
				return nil, fmt.Errorf("Couldn't parse measurement timestamp: %s", err)
			}
			ret = append(ret, HourlyEnergyMeasurement{
				Timestamp:             ts,
				MeteringPointId:       meteringPointId,
				MeteringPointLocation: meteringPointLocation,
				Value: v.Values.EnergyConsumption.Value,
			})
		}

	} // Meteringpoint loop
	return ret, nil
}

func (self *CarunaClient) Logout() error {
	resp, err := self.GetPage(CarunaApiLogout)
	if err != nil {
		return err
	}

	// Logout from SSO portal as well
	url, err := resp.OrigResponse.Request.URL.Parse(CarunaSSOLogout)
	if err != nil {
		return err
	}
	_, err = self.GetPage(url.String())
	if err != nil {
		return err
	}
	return nil
}

func (self *CarunaClient) Authenticate(username, password string) error {
	resp, err := self.GetPage(self.AuthUrl)
	if err != nil {
		return err
	}

	self.Logger.Println("Finding login form..")
	loginForm, err := parser.FindLoginForm(bytes.NewReader(resp.Data), &parser.FormQuery{Id: CarunaLoginFormId})
	if err != nil {
		return err
	}

	// Find out the absolute url for the form action
	actionURL := resp.OrigResponse.Request.URL.ResolveReference(loginForm.ActionURL)

	// Set username and password
	loginForm.FormValues.Set(CarunaLoginFieldUsername, username)
	loginForm.FormValues.Set(CarunaLoginFieldPassword, password)

	// Do login
	resp, err = self.PostPage(actionURL.String(), loginForm.FormValues)
	if err != nil {
		return err
	}

	// We'll need to perform postback for SSO authentication so let's find the form first
	postBackForm, err := parser.FindLoginForm(bytes.NewReader(resp.Data), nil)
	if err != nil {
		return err
	}
	// Now do postback
	actionURL = resp.OrigResponse.Request.URL.ResolveReference(postBackForm.ActionURL)
	resp, err = self.PostPage(actionURL.String(), postBackForm.FormValues)
	if err != nil {
		return err
	}

	// Store last url for future reference
	self.refUrl = resp.OrigResponse.Request.URL

	self.CustomerInfo, err = self.GetCustomerInfo()
	if err != nil {
		return fmt.Errorf("Could not get Customer Info. Wrong credentials?")
	}

	return nil
}

func (self *CarunaClient) SetLogger(logger *log.Logger) {
	self.Logger = logger
	self.Logger.SetPrefix("[CarunaClient] ")
}

func NewCarunaClient(urlStr, username, password string, opts *ClientOpts) (*CarunaClient, error) {
	client := &CarunaClient{}

	if opts.Logger == nil {
		client.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	} else {
		client.SetLogger(opts.Logger)
	}

	jar, _ := cookiejar.New(nil)

	client.Client = &http.Client{
		Jar: jar,
	}

	client.AuthUrl = urlStr

	if err := client.Authenticate(username, password); err != nil {
		return nil, err
	}

	return client, nil
}
