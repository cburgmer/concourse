package engine

import (
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager"

	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/db"
	"github.com/concourse/concourse/atc/event"
	"github.com/concourse/concourse/atc/exec"
)

type getDelegate struct {
	exec.BuildStepDelegate

	build       db.Build
	eventOrigin event.Origin
}

func NewGetDelegate(build db.Build, planID atc.PlanID, clock clock.Clock) exec.GetDelegate {
	return &getDelegate{
		BuildStepDelegate: NewBuildStepDelegate(build, planID, clock),

		build: build,
		eventOrigin: event.Origin{
			ID: event.OriginID(planID),
		},
	}
}

func (d *getDelegate) Finished(logger lager.Logger, exitStatus exec.ExitStatus, version atc.Version) {
	err := d.build.SaveEvent(event.FinishGet{
		Origin:         d.eventOrigin,
		ExitStatus:     int(exitStatus),
		FetchedVersion: version,
	})
	if err != nil {
		logger.Error("failed-to-save-finish-get-event", err)
		return
	}

	logger.Info("finished", lager.Data{"exit-status": exitStatus})
}
