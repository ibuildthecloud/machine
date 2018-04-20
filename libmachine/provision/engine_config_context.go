package provision

import (
	"github.com/docker/machine/libmachine/engine"
)

type EngineConfigContext struct {
	EngineOptions    engine.Options
	DockerOptionsDir string
}
