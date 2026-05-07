package redfish

type ThermalInfo struct {
	Temperatures []TempSensor `json:"Temperatures"`
	Fans         []FanSensor  `json:"Fans"`
}

type TempSensor struct {
	Name              string  `json:"Name"`
	ReadingCelsius    float64 `json:"ReadingCelsius"`
	UpperThresholdCritical float64 `json:"UpperThresholdCritical"`
	Status            struct {
		Health string `json:"Health"`
		State  string `json:"State"`
	} `json:"Status"`
}

type FanSensor struct {
	Name         string  `json:"Name"`
	Reading      float64 `json:"Reading"`
	ReadingUnits string  `json:"ReadingUnits"`
	Status       struct {
		Health string `json:"Health"`
		State  string `json:"State"`
	} `json:"Status"`
}

func (c *Client) GetThermal() (*ThermalInfo, error) {
	var t ThermalInfo
	if err := c.get("/Chassis/System.Embedded.1/Thermal", &t); err != nil {
		return nil, err
	}
	return &t, nil
}
