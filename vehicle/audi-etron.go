package vehicle

import (
	"context"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/api/store"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"
	"github.com/evcc-io/evcc/vehicle/audi/etron"
	"github.com/evcc-io/evcc/vehicle/vag/aazsproxy"
	"github.com/evcc-io/evcc/vehicle/vag/idkproxy"
	"github.com/evcc-io/evcc/vehicle/vag/service"
	"github.com/evcc-io/evcc/vehicle/vw/id"
)

// https://github.com/TA2k/ioBroker.vw-connect
// https://github.com/arjenvrh/audi_connect_ha/blob/master/custom_components/audiconnect/audi_services.py

// Etron is an api.Vehicle implementation for Audi eTron cars
type Etron struct {
	*embed
	*id.Provider // provides the api implementations
}

func init() {
	registry.AddWithStore("etron", NewEtronFromConfig)
}

// NewEtronFromConfig creates a new vehicle
func NewEtronFromConfig(factory store.Provider, other map[string]interface{}) (api.Vehicle, error) {
	cc := struct {
		embed               `mapstructure:",squash"`
		User, Password, VIN string
		Cache               time.Duration
		Timeout             time.Duration
	}{
		Cache:   interval,
		Timeout: request.Timeout,
	}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}

	v := &Etron{
		embed: &cc.embed,
	}

	log := util.NewLogger("etron").Redact(cc.User, cc.Password, cc.VIN)

	idkStore := factory("audi.etron.tokens.idk." + cc.User)
	idk := idkproxy.New(log, etron.IDKParams).WithStore(idkStore)

	azsStore := factory("audi.etron.tokens.azs." + cc.User)
	azs := aazsproxy.New(log).WithStore(azsStore)

	ats, its, err := service.AAZSTokenSource(log, idk, azs, etron.AZSConfig, etron.AuthParams, cc.User, cc.Password)
	if err != nil {
		return nil, err
	}

	// use the etron API for list of vehicles
	api := etron.NewAPI(log, ats)

	vehicle, err := ensureVehicleEx(
		cc.VIN, func() ([]etron.Vehicle, error) {
			ctx, cancel := context.WithTimeout(context.Background(), cc.Timeout)
			defer cancel()
			return api.Vehicles(ctx)
		},
		func(v etron.Vehicle) string {
			return v.VIN
		},
	)

	if err == nil {
		if v.Title_ == "" {
			v.Title_ = vehicle.Nickname
		}

		api := id.NewAPI(log, its)
		api.Client.Timeout = cc.Timeout

		v.Provider = id.NewProvider(api, vehicle.VIN, cc.Cache)
	}

	return v, err
}
