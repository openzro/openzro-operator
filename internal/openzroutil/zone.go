package openzroutil

import (
	"context"
	"fmt"
	"slices"

	openzro "github.com/openzro/openzro/shared/management/client/rest"
	"github.com/openzro/openzro/shared/management/http/api"
)

func GetDNSZoneByName(ctx context.Context, nbClient *openzro.Client, name string) (api.Zone, error) {
	resp, err := nbClient.DNSZones.ListZones(ctx)
	if err != nil {
		return api.Zone{}, err
	}
	zoneIdx := slices.IndexFunc(resp, func(zone api.Zone) bool {
		return zone.Name == name
	})
	if zoneIdx == -1 {
		return api.Zone{}, fmt.Errorf("zone with name %s cannot be found", name)
	}
	return resp[zoneIdx], nil
}
