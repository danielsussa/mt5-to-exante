package exchanges

import (
	"fmt"
	"github.com/goccy/go-yaml"
	"os"
)

type Api struct {
	data data
}

type data struct {
	Description string          `yaml:"description"`
	Exchanges   []DataExchanges `json:"exchanges"`
}

type DataExchanges struct {
	Exante     string  `yaml:"exante"`
	MetaTrader string  `yaml:"metaTrader"`
	PriceStep  float64 `yaml:"priceStep"`
}

func New(path string) (*Api, error) {
	dat, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot find exchange file")
	}

	var d data
	err = yaml.Unmarshal(dat, &d)
	if err != nil {
		return nil, fmt.Errorf("error to convert exchange file: %s", err.Error())
	}

	return &Api{data: d}, err
}

func (a Api) GetByMTValue(mtval string) (DataExchanges, bool) {
	for _, d := range a.data.Exchanges {
		if d.MetaTrader == mtval {
			return d, true
		}
	}

	return DataExchanges{}, false
}
