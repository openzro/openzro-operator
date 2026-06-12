package openzroutil

import (
	"context"
	"fmt"
	"slices"

	openzro "github.com/openzro/openzro/management/client/rest"
	"github.com/openzro/openzro/management/server/http/api"
)

func GetDNSZoneByName(ctx context.Context, nbClient *openzro.Client, name string) (api.DNSZone, error) {
	resp, err := nbClient.DNSZones.ListZones(ctx)
	if err != nil {
		return api.DNSZone{}, err
	}
	zoneIdx := slices.IndexFunc(resp, func(zone api.DNSZone) bool {
		return zone.Name == name
	})
	if zoneIdx == -1 {
		return api.DNSZone{}, fmt.Errorf("zone with name %s cannot be found", name)
	}
	return resp[zoneIdx], nil
}
