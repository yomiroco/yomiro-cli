// Package svcwrap wraps kardianos/service to install, start, stop, and
// uninstall the gateway as a launchd / systemd / SCM service.
package svcwrap

import (
	"os"
	"path/filepath"

	"github.com/kardianos/service"
)

const (
	serviceName        = "io.yomiro.gw"
	serviceDisplayName = "Yomiro Gateway"
	serviceDescription = "Customer-side gateway daemon for the Yomiro platform"
)

// Manager wraps a kardianos/service.Service for our daemon binary.
type Manager struct {
	svc service.Service
}

// New returns a Manager configured to run "yomiro gw run" as a service.
// daemonStart/daemonStop run on the daemon side; for the CLI control side,
// pass nil — we only call Install/Start/Stop/Uninstall.
func New() (*Manager, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cfg := &service.Config{
		Name:             serviceName,
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		Executable:       exe,
		Arguments:        []string{"gw", "run"},
		WorkingDirectory: filepath.Dir(exe),
		Option: service.KeyValue{
			"KeepAlive":   true,
			"RunAtLoad":   true,
			"UserService": true, // launchd LaunchAgent (per-user, not LaunchDaemon)
		},
	}
	svc, err := service.New(noopProgram{}, cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{svc: svc}, nil
}

func (m *Manager) Install() error                  { return m.svc.Install() }
func (m *Manager) Uninstall() error                { return m.svc.Uninstall() }
func (m *Manager) Start() error                    { return m.svc.Start() }
func (m *Manager) Stop() error                     { return m.svc.Stop() }
func (m *Manager) Status() (service.Status, error) { return m.svc.Status() }

// noopProgram exists so service.New doesn't panic when the CLI side just
// wants to call Install/Start without running anything.
type noopProgram struct{}

func (noopProgram) Start(_ service.Service) error { return nil }
func (noopProgram) Stop(_ service.Service) error  { return nil }
