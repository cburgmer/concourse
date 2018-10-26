package v2

import (
	"bytes"
	"context"
	"encoding/json"

	"code.cloudfoundry.org/garden"
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/worker"
)

type getRequest struct {
	Config  map[string]interface{} `json:"config"`
	Space   atc.Space              `json:"space"`
	Version atc.Version            `json:"version,omitempty"`
}

func (r *resource) Get(
	ctx context.Context,
	volume worker.Volume,
	ioConfig atc.IOConfig,
	source atc.Source,
	params atc.Params,
	space atc.Space,
	version atc.Version,
) error {
	config := source
	for k, v := range params {
		config[k] = v
	}

	input := getRequest{config, space, version}
	request, err := json.Marshal(input)
	if err != nil {
		return err
	}

	_, err = r.container.Property(resourceResultPropertyName)
	if err == nil {
		return err
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	processIO := garden.ProcessIO{
		Stdin:  bytes.NewBuffer(request),
		Stdout: stdout,
	}

	if ioConfig.Stderr != nil {
		processIO.Stderr = ioConfig.Stderr
	} else {
		processIO.Stderr = stderr
	}

	var process garden.Process

	process, err = r.container.Attach(TaskProcessID, processIO)
	if err != nil {
		process, err = r.container.Run(garden.ProcessSpec{
			ID:   TaskProcessID,
			Path: r.info.Artifacts.Get,
			Args: []string{atc.ResourcesDir("get")},
		}, processIO)
		if err != nil {
			return err
		}
	}

	processExited := make(chan struct{})

	var processStatus int
	var processErr error

	go func() {
		processStatus, processErr = process.Wait()
		close(processExited)
	}()

	select {
	case <-processExited:
		if processErr != nil {
			return processErr
		}

		if processStatus != 0 {
			return ErrResourceScriptFailed{
				Path:       r.info.Artifacts.Get,
				Args:       []string{atc.ResourcesDir("get")},
				ExitStatus: processStatus,

				Stderr: stderr.String(),
			}
		}

		err := r.container.SetProperty(resourceResultPropertyName, stdout.String())
		if err != nil {
			return err
		}

		return nil

	case <-ctx.Done():
		r.container.Stop(false)
		<-processExited
		return ctx.Err()
	}

	return nil
}
