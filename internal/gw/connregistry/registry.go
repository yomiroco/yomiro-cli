// Package connregistry constructs a connectors.TargetResolver with the
// built-in connectors enabled. It lives outside the connectors package to
// avoid an import cycle (each connector imports the parent connectors
// package for its types).
package connregistry

import (
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors/generic"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors/modbus"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors/mqtt"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors/opcua"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors/otel"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors/sonos"
)

// Build constructs a TargetResolver with the named connectors enabled.
// "generic" is always loaded as the fallback.
func Build(enabled []string) *connectors.TargetResolver {
	r := connectors.NewResolver()
	r.SetGeneric(generic.New())
	for _, name := range enabled {
		switch name {
		case "mqtt":
			r.Register("mqtt-broker", mqtt.New(), nil)
		case "modbus":
			r.Register("modbus-tcp", modbus.New(), nil)
		case "opcua":
			r.Register("opcua", opcua.New(), nil)
		case "sonos":
			s := sonos.New()
			r.Register("sonos_speaker", s, s)
		case "otel":
			r.Register("otel-collector", otel.New(), nil)
		}
	}
	return r
}
